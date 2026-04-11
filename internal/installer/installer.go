package installer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/etkecc/go-kit/workpool"

	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/runner"
)

const (
	// RetriesMax is the maximum number of retries for git clone operations
	RetriesMax = 5
	// RetryStepDelay is the delay between retry attempts (exponential backoff)
	RetryStepDelay = 1 * time.Second
)

var ignoredVersions = map[string]bool{
	"main":   true,
	"master": true,
}

// Progress represents the installation status of a single role.
type Progress struct {
	Name       string
	Version    string
	OldVersion string
	Status     string // "active" | "done" | "skipped" | "error"
	Log        string // verbose log line (non-empty only when verbose mode is on)
	Err        error
}

// Installer handles installing and managing Ansible roles from a requirements.yml file.
// It uses a Runner to execute git commands and an fs.FS for reading role metadata.
type Installer struct {
	runner    runner.Runner
	fsys      fs.FS
	rolesPath string
	limit     int
	cleanup   bool
}

// New creates a new Installer
func New(r runner.Runner, rolesPath string, limit int, cleanup bool) *Installer {
	return &Installer{
		runner:    r,
		fsys:      os.DirFS(rolesPath),
		rolesPath: rolesPath,
		limit:     limit,
		cleanup:   cleanup,
	}
}

// FS returns the filesystem used for reading role metadata
func (i *Installer) FS() fs.FS {
	return i.fsys
}

// InstallMissing writes all roles to the target roles dir if role doesn't exist or has different version.
// Progress events are sent to the progress channel (if non-nil); the channel is closed when all installs complete.
func (i *Installer) InstallMissing(entries models.File, progress chan<- Progress) error {
	if err := i.bootstrapRoles(); err != nil {
		return err
	}
	// Refresh FS after potentially creating the roles directory.
	// Take a local snapshot before the concurrent loop — goroutines read from
	// this snapshot; i.fsys is refreshed once after all installations complete.
	i.fsys = os.DirFS(i.rolesPath)
	fsys := i.fsys

	rolesLen := entries.RolesLen()
	limit := i.limit
	if limit == 0 {
		limit = rolesLen
	}
	wp := workpool.New(limit)
	var (
		mu      sync.Mutex
		changes models.UpdatedItems
		errs    []error
	)

	for _, entry := range entries {
		if entry.Include != "" { // skip entries with include directive
			continue
		}
		wp.Do(func() {
			i.installEntry(entry, fsys, &mu, &changes, &errs, progress)
		})
	}
	wp.Run()
	i.fsys = os.DirFS(i.rolesPath)

	if progress != nil {
		close(progress)
	}

	if len(errs) == 0 {
		return nil
	}
	errStrs := make([]string, 0, len(errs))
	for _, err := range errs {
		errStrs = append(errStrs, err.Error())
	}
	return errors.New(strings.Join(errStrs, "\n"))
}

// installEntry executes a single role install inside the workpool goroutine.
func (i *Installer) installEntry(entry *models.Entry, fsys fs.FS, mu *sync.Mutex, changes *models.UpdatedItems, errs *[]error, progress chan<- Progress) {
	if progress != nil {
		progress <- Progress{Name: entry.GetName(), Version: entry.Version, Status: "active"}
	}
	oldVersion, installed, logLine, err := i.processEntry(entry, fsys)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		*errs = append(*errs, err)
		if progress != nil {
			progress <- Progress{Name: entry.GetName(), Version: entry.Version, Status: "error", Log: logLine, Err: err}
		}
		return
	}
	if installed && !ignoredVersions[entry.Version] {
		*changes = changes.Add(entry.GetName(), oldVersion, entry.Version)
	}
	if progress == nil {
		return
	}
	if installed {
		progress <- Progress{Name: entry.GetName(), Version: entry.Version, OldVersion: oldVersion, Status: "done", Log: logLine}
	} else {
		progress <- Progress{Name: entry.GetName(), Version: entry.Version, Status: "skipped"}
	}
}

// processEntry checks and installs a single role.
// Returns the previously installed version, whether the role was installed/updated, a verbose log line, and any error.
func (i *Installer) processEntry(entry *models.Entry, fsys fs.FS) (oldVersion string, installed bool, logLine string, err error) {
	if entry.IsInstalled(fsys) {
		return "", false, "", nil
	}
	existingInfo, _ := entry.GetInstallInfo(fsys) //nolint:errcheck // parse failure → empty version → unknown old version, will reinstall
	oldVersion = existingInfo.Version
	ok, logLine, err := i.installRole(entry)
	if err != nil {
		return "", false, logLine, fmt.Errorf("installing %s@%s: %w", entry.GetName(), entry.Version, err)
	}
	return oldVersion, ok, logLine, nil
}

// GetInstalled returns all roles that are already installed
func (i *Installer) GetInstalled(entries models.File) models.File {
	installed := models.File{}
	for _, entry := range entries {
		info, _ := entry.GetInstallInfo(i.fsys) //nolint:errcheck // parse failure → empty version → not listed as installed
		if info.Version != "" {
			installed = append(installed, entry)
		}
	}
	return installed
}

// installRole writes specific role version to the target roles dir.
// Returns whether the role was installed, a verbose log line, and any error.
func (i *Installer) installRole(entry *models.Entry) (installed bool, log string, err error) {
	name := entry.GetName()

	repo := strings.Replace(entry.Src, "git+", "", 1)
	tmpdir, err := os.MkdirTemp("", "agru-"+name+"-*")
	if err != nil {
		return false, "", fmt.Errorf("creating tmp dir: %w", err)
	}
	tmpfile := tmpdir + ".tar"
	if i.cleanup {
		defer i.cleanupRole(tmpdir, tmpfile)
	}

	// clone repo
	var clone strings.Builder
	clone.WriteString("git clone -q --depth 1 ")
	// git commit
	if len(entry.Version) >= 40 {
		clone.WriteString("-c remote.origin.fetch=+")
		clone.WriteString(entry.Version)
		clone.WriteString(":refs/remotes/origin/")
		clone.WriteString(entry.Version)
	} else { // git tag
		clone.WriteString("-b ")
		clone.WriteString(entry.Version)
	}
	clone.WriteString(" ")
	clone.WriteString(repo)
	clone.WriteString(" ")
	clone.WriteString(tmpdir)

	logLine := fmt.Sprintf("[%s] cloning %s @ %s", name, repo, entry.Version)
	out, err := i.runClone(clone.String(), 0)
	if err != nil {
		return false, logLine, fmt.Errorf("cloning repo: %w\n%s", err, out)
	}

	sha, err := i.runner.Run("git rev-parse HEAD", tmpdir)
	if err != nil {
		return false, logLine, fmt.Errorf("getting commit hash: %w", err)
	}
	logLine = fmt.Sprintf("[%s] cloned %s @ %s (sha: %s)", name, repo, entry.Version, sha)

	// check if the role is already installed
	cachedInfo, _ := entry.GetInstallInfo(i.fsys) //nolint:errcheck // parse failure → empty commit → will reinstall
	installedCommit := cachedInfo.InstallCommit
	if sha != "" && installedCommit != "" && sha == installedCommit {
		return false, logLine, nil
	}

	// create archive from the cloned source
	var archive strings.Builder
	archive.WriteString("git archive --prefix=")
	archive.WriteString(name)
	archive.WriteString("/ --output=")
	archive.WriteString(tmpfile)
	archive.WriteString(" ")
	archive.WriteString(entry.Version)
	out, err = i.runner.Run(archive.String(), tmpdir)
	if err != nil {
		return false, logLine, fmt.Errorf("archiving repo: %w\n%s", err, out)
	}

	// remove existing role directory to ensure stale files from previous versions are cleaned up
	if err := os.RemoveAll(path.Join(i.rolesPath, name)); err != nil {
		return false, logLine, fmt.Errorf("removing existing role dir: %w", err)
	}

	// extract the archive into roles path
	out, err = i.runner.Run("tar -xf "+tmpfile, i.rolesPath)
	if err != nil {
		return false, logLine, fmt.Errorf("extracting archive: %w\n%s", err, out)
	}

	// write install info file
	outb, err := entry.GenerateInstallInfo(sha)
	if err != nil {
		return false, logLine, fmt.Errorf("generating install info: %w", err)
	}
	if err := os.WriteFile(path.Join(i.rolesPath, name, "meta", ".galaxy_install_info"), outb, 0o600); err != nil {
		return false, logLine, fmt.Errorf("writing install info: %w", err)
	}

	return true, logLine, nil
}

// runClone runs git clone with exponential-backoff retry on network failures
func (i *Installer) runClone(cmd string, attempt int) (string, error) {
	out, err := i.runner.Run(cmd, "")
	if err == nil {
		return out, nil
	}

	// fatal: unable to access 'https://github.com/user/repo.git/': Failed to connect to github.com port 443 after 135428 ms: Couldn't connect to server
	if strings.Contains(out, "Couldn't connect to server") && attempt < RetriesMax {
		delay := RetryStepDelay * time.Duration(attempt)
		time.Sleep(delay)
		return i.runClone(cmd, attempt+1)
	}

	return out, err
}

// bootstrapRoles creates the roles directory if it doesn't exist
func (i *Installer) bootstrapRoles() error {
	_, err := os.Stat(i.rolesPath)
	if err != nil && os.IsNotExist(err) {
		mkerr := os.Mkdir(i.rolesPath, 0o700)
		if mkerr != nil {
			return fmt.Errorf("creating roles path: %w", mkerr)
		}
	}
	return nil
}

// cleanupRole removes all temporary dirs and files created during role installation
func (i *Installer) cleanupRole(tmpdir, tmpfile string) {
	os.RemoveAll(tmpdir)
	os.Remove(tmpfile)
}
