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

// Installer installs and manages Ansible roles and collections from a requirements.yml file.
type Installer struct {
	runner    runner.Runner
	fsys      fs.FS
	rolesPath string
	collPath  string
	collFS    fs.FS
	limit     int
	cleanup   bool
}

// New creates a new Installer
func New(r runner.Runner, rolesPath, collectionsPath string, limit int, cleanup bool) *Installer {
	return &Installer{
		runner:    r,
		fsys:      os.DirFS(rolesPath),
		rolesPath: rolesPath,
		collPath:  collectionsPath,
		collFS:    os.DirFS(collectionsPath),
		limit:     limit,
		cleanup:   cleanup,
	}
}

// FS returns the filesystem used for reading role metadata
func (i *Installer) FS() fs.FS {
	return i.fsys
}

// InstallMissing writes roles and collections missing or outdated; closes progress (if non-nil) when done.
func (i *Installer) InstallMissing(entries models.File, colls models.Collections, progress chan<- Progress) error {
	if progress != nil {
		defer close(progress)
	}
	if err := i.bootstrapRoles(); err != nil {
		return err
	}
	if err := i.bootstrapCollections(); err != nil {
		return err
	}
	// dirs exist now: refresh fsys, then snapshot locally.
	i.fsys = os.DirFS(i.rolesPath)
	i.collFS = os.DirFS(i.collPath)

	wp := i.newWorkpool(entries.RolesLen() + len(colls))
	var (
		mu      sync.Mutex
		changes models.UpdatedItems
		errs    []error
	)
	i.submitRoles(wp, entries, i.fsys, &mu, &changes, &errs, progress)
	i.submitCollections(wp, colls, &mu, &changes, &errs, progress)
	wp.Run()

	i.fsys = os.DirFS(i.rolesPath)
	i.collFS = os.DirFS(i.collPath)

	if len(errs) == 0 {
		return nil
	}
	errStrs := make([]string, 0, len(errs))
	for _, err := range errs {
		errStrs = append(errStrs, err.Error())
	}
	return errors.New(strings.Join(errStrs, "\n"))
}

// newWorkpool creates a workpool with a limit derived from the total item count.
func (i *Installer) newWorkpool(totalItems int) *workpool.WorkPool {
	limit := i.limit
	if limit == 0 {
		limit = totalItems
	}
	if limit < 1 {
		limit = 1
	}
	return workpool.New(limit)
}

// submitRoles submits role install jobs to the workpool.
func (i *Installer) submitRoles(wp *workpool.WorkPool, entries models.File, fsys fs.FS, mu *sync.Mutex, changes *models.UpdatedItems, errs *[]error, progress chan<- Progress) {
	for _, entry := range entries {
		if entry.Include != "" || entry.Unsupported() != "" {
			if entry.Unsupported() != "" && progress != nil {
				progress <- Progress{Name: entry.GetName(), Status: "unsupported", Log: entry.Unsupported()}
			}
			continue
		}
		wp.Do(func() {
			i.installEntry(entry, fsys, mu, changes, errs, progress)
		})
	}
}

// submitCollections submits collection install jobs to the workpool.
func (i *Installer) submitCollections(wp *workpool.WorkPool, colls models.Collections, mu *sync.Mutex, changes *models.UpdatedItems, errs *[]error, progress chan<- Progress) {
	for _, col := range colls {
		if col.Unsupported() != "" {
			if progress != nil {
				progress <- Progress{Name: col.GetFQCN(), Status: "unsupported", Log: col.Unsupported()}
			}
			continue
		}
		wp.Do(func() {
			i.installCollection(col, mu, changes, errs, progress)
		})
	}
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

// processEntry checks and installs a single role, returning its prior version, whether it changed, and a log line.
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

// installRole writes the specific role version to the target roles dir, returning whether it installed and a log line.
func (i *Installer) installRole(entry *models.Entry) (installed bool, log string, err error) {
	name := entry.GetName()
	if !models.IsValidRoleName(name) {
		return false, "", fmt.Errorf("invalid role name %q", name)
	}

	repo := strings.Replace(entry.Src, "git+", "", 1)

	// Validate URL and version: no spaces, no leading dash (RCE guard).
	if err := models.ValidateGitArg(repo); err != nil {
		return false, "", fmt.Errorf("invalid role repo URL %q: %w", repo, err)
	}
	if entry.Version != "" {
		if err := models.ValidateGitArg(entry.Version); err != nil {
			return false, "", fmt.Errorf("invalid role version %q: %w", entry.Version, err)
		}
	}

	logLine := fmt.Sprintf("[%s] cloning %s @ %s", name, repo, entry.Version)
	tmpdir, tmpfile, sha, err := i.cloneRole(name, repo, entry.Version)
	if i.cleanup {
		defer i.cleanupRole(tmpdir, tmpfile)
	}
	if err != nil {
		return false, logLine, err
	}
	logLine = fmt.Sprintf("[%s] cloned %s @ %s (sha: %s)", name, repo, entry.Version, sha)

	// check if the role is already installed
	cachedInfo, _ := entry.GetInstallInfo(i.fsys) //nolint:errcheck // parse failure → empty commit → will reinstall
	installedCommit := cachedInfo.InstallCommit
	if sha != "" && installedCommit != "" && sha == installedCommit {
		return false, logLine, nil
	}

	// SHA-pinned clones check out HEAD, so the archive tree may differ; the write sink is guarded by assertRegularDir.
	if err := verifyRoleTree(tmpdir); err != nil {
		return false, logLine, fmt.Errorf("unsafe role content: %w", err)
	}

	// create archive from the cloned source
	archiveArgs := []string{"git", "archive", "--prefix=" + name + "/", "--output=" + tmpfile, entry.Version}
	out, err := i.runner.RunArgs(archiveArgs, tmpdir)
	if err != nil {
		return false, logLine, fmt.Errorf("archiving repo: %w\n%s", err, out)
	}

	// remove existing role directory to ensure stale files from previous versions are cleaned up
	if err := RemoveRoleDir(path.Join(i.rolesPath, name)); err != nil {
		return false, logLine, err
	}

	// extract the archive into roles path
	tarArgs := []string{"tar", "-xf", tmpfile}
	out, err = i.runner.RunArgs(tarArgs, i.rolesPath)
	if err != nil {
		return false, logLine, fmt.Errorf("extracting archive: %w\n%s", err, out)
	}

	// write install info file
	if err := i.writeRoleInstallInfo(name, entry, sha); err != nil {
		return false, logLine, err
	}

	return true, logLine, nil
}

// writeRoleInstallInfo validates the meta dir and writes .galaxy_install_info under it.
func (i *Installer) writeRoleInstallInfo(name string, entry *models.Entry, sha string) error {
	if err := assertRegularDir(path.Join(i.rolesPath, name, "meta")); err != nil {
		return fmt.Errorf("refusing to write install info: %w", err)
	}
	infoPath := path.Join(i.rolesPath, name, "meta", ".galaxy_install_info")
	// Remove first: unlinks a repo-supplied symlink without following it, like the collection manifest write.
	if err := os.Remove(infoPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing existing install info: %w", err)
	}
	outb, err := entry.GenerateInstallInfo(sha)
	if err != nil {
		return fmt.Errorf("generating install info: %w", err)
	}
	if err := os.WriteFile(infoPath, outb, 0o600); err != nil {
		return fmt.Errorf("writing install info: %w", err)
	}
	return nil
}

// roleCloneArgs builds a safe argv slice for git clone of a role repo.
func roleCloneArgs(repo, version, tmpdir string) []string {
	args := []string{"git", "clone", "-q", "--depth", "1"}
	if version != "" {
		if len(version) >= 40 {
			args = append(args, "-c", "remote.origin.fetch=+"+version+":refs/remotes/origin/"+version)
		} else {
			args = append(args, "-b", version)
		}
	}
	return append(args, "--", repo, tmpdir)
}

// runCloneArgs runs git clone via RunArgs with exponential-backoff retry on network failures.
func (i *Installer) runCloneArgs(args []string, attempt int) (string, error) {
	out, err := i.runner.RunArgs(args, "")
	if err == nil {
		return out, nil
	}

	if strings.Contains(out, "Couldn't connect to server") && attempt < RetriesMax {
		delay := RetryStepDelay * time.Duration(attempt)
		time.Sleep(delay)
		return i.runCloneArgs(args, attempt+1)
	}

	return out, err
}

// cloneRole clones the role repo into a fresh tmpdir and returns the dir, tar path, and HEAD sha.
func (i *Installer) cloneRole(name, repo, version string) (tmpdir, tmpfile, sha string, err error) {
	tmpdir, err = os.MkdirTemp("", "agru-"+name+"-*")
	if err != nil {
		return "", "", "", fmt.Errorf("creating tmp dir: %w", err)
	}
	tmpfile = tmpdir + ".tar"
	out, err := i.runCloneArgs(roleCloneArgs(repo, version, tmpdir), 0)
	if err != nil {
		return tmpdir, tmpfile, "", fmt.Errorf("cloning repo: %w\n%s", err, out)
	}
	// >=40 clones check out the default branch; switch to the pinned ref so the tree matches git archive.
	if len(version) >= 40 {
		out, err = i.runner.RunArgs([]string{"git", "checkout", "-q", version}, tmpdir)
		if err != nil {
			return tmpdir, tmpfile, "", fmt.Errorf("checking out pinned version: %w\n%s", err, out)
		}
	}
	sha, err = i.runner.RunArgs([]string{"git", "rev-parse", "HEAD"}, tmpdir)
	if err != nil {
		return tmpdir, tmpfile, "", fmt.Errorf("getting commit hash: %w", err)
	}
	return tmpdir, tmpfile, sha, nil
}

// bootstrapRoles creates the roles dir (0700, it can hold secrets) and its missing parents (0755).
func (i *Installer) bootstrapRoles() error {
	dir := path.Clean(i.rolesPath)
	if err := os.MkdirAll(path.Dir(dir), 0o755); err != nil {
		return fmt.Errorf("creating roles path: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("creating roles path: %w", err)
	}
	return nil
}

// cleanupRole removes all temporary dirs and files created during role installation
func (i *Installer) cleanupRole(tmpdir, tmpfile string) {
	os.RemoveAll(tmpdir)
	os.Remove(tmpfile)
}
