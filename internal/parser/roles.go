package parser

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/etkecc/go-kit/workpool"

	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/utils"
)

const (
	// RetriesMax is the maximum number of retries for git operations
	RetriesMax = 5
	// RetryStepDelay is the delay between retry attempts (exponential backoff)
	RetryStepDelay = 1 * time.Second
)

var ignoredVersions = map[string]bool{
	"main":   true,
	"master": true,
}

// InstallMissingRoles writes all roles to the target roles dir if role doesn't exist or has different version
//
//nolint:gocognit // TODO: refactor
func InstallMissingRoles(rolesPath string, entries models.File, limit int, cleanup bool) error {
	if err := bootstrapRoles(rolesPath); err != nil {
		return err
	}

	rolesLen := entries.RolesLen()
	// if limit is 0, then no limit
	if limit == 0 {
		limit = rolesLen
	}
	bar := utils.NewProgressbar(rolesLen, "installing roles")
	wp := workpool.New(limit)
	changes := models.UpdatedItems{}
	errchan := make(chan error, rolesLen)
	errs := []error{}

	go func(errchan chan error) {
		for err := range errchan {
			errs = append(errs, err)
		}
	}(errchan)

	for _, entry := range entries {
		if entry.Include != "" { // skip entries with include directive
			continue
		}
		wp.Do(func() {
			defer bar.Add(1) //nolint:errcheck // don't care about error here

			if entry.IsInstalled(rolesPath) {
				return
			}

			oldVersion := entry.GetInstallInfo(rolesPath).Version
			ok, err := installRole(rolesPath, entry, cleanup)
			if ok {
				bar.AddDetail(fmt.Sprintf("installed %s@%s", entry.GetName(), entry.Version)) //nolint:errcheck // don't care about error here
			}
			if err != nil {
				errchan <- fmt.Errorf("installing %s@%s: %w", entry.GetName(), entry.Version, err)
				bar.AddDetail(fmt.Sprintf("failed %s@%s", entry.GetName(), entry.Version)) //nolint:errcheck // don't care about error here
				return
			}
			if !ignoredVersions[entry.Version] {
				changes = changes.Add(entry.GetName(), oldVersion, entry.Version)
			}
		})
	}
	wp.Run()

	if len(changes) > 0 {
		utils.Log(changes.String("roles updated:\n"))
	}
	close(errchan)
	if len(errs) == 0 {
		return nil
	}

	errStrs := []string{}
	for _, err := range errs {
		errStrs = append(errStrs, err.Error())
	}
	return errors.New(strings.Join(errStrs, "\n"))
}

// GetInstalledRoles returns all roles that are already installed
func GetInstalledRoles(rolesPath string, entries models.File) models.File {
	installed := models.File{}
	for _, entry := range entries {
		if entry.GetInstallInfo(rolesPath).Version != "" {
			installed = append(installed, entry)
		}
	}
	return installed
}

// getNewVersion checks for newer git tag available on the src's remote
func getNewVersion(src, version string) (string, error) {
	if ignoredVersions[version] {
		return "", nil
	}

	// not a git repo
	if !strings.Contains(src, "git") {
		return "", nil
	}

	repo := strings.Replace(src, "git+https", "https", 1)
	tags, err := utils.Run("git ls-remote -tq --sort=-version:refname "+repo, "")
	if err != nil {
		return "", fmt.Errorf("running git ls-remote: %w", err)
	}
	if tags == "" {
		return "", nil
	}

	lastline := strings.Split(tags, "\n")[0]
	tagidx := strings.Index(lastline, "refs/tags/")
	if tagidx == -1 {
		return "", fmt.Errorf("cannot find tag in git ls-remote output, lastline: %s", lastline)
	}
	last := strings.Replace(lastline[tagidx:], "refs/tags/", "", 1)
	last = strings.Replace(last, "^{}", "", 1) // NOTE: very weird case with some github repos, didn't find out why it does that
	if last != version {
		return last, nil
	}

	return "", nil
}

// cleanupRole removes all temporary dirs and files created during role installation
func cleanupRole(tmpdir, tmpfile string) {
	os.RemoveAll(tmpdir)
	os.Remove(tmpfile)
}

func runClone(cmd string, currentAttempt ...int) (string, error) {
	attempt := 0
	if len(currentAttempt) > 0 {
		attempt = currentAttempt[0]
	}

	out, err := utils.Run(cmd, "")
	if err == nil {
		return out, nil
	}

	// fatal: unable to access 'https://github.com/user/repo.git/': Failed to connect to github.com port 443 after 135428 ms: Couldn't connect to server
	if strings.Contains(out, "Couldn't connect to server") && attempt < RetriesMax {
		delay := RetryStepDelay * time.Duration(attempt)
		time.Sleep(delay)
		return runClone(cmd, attempt+1)
	}

	return out, err
}

// installRole writes specific role version to the target roles dir
func installRole(rolesPath string, entry *models.Entry, cleanup bool) (bool, error) {
	name := entry.GetName()

	repo := strings.Replace(entry.Src, "git+", "", 1)
	tmpdir, err := os.MkdirTemp("", "agru-"+name+"-*")
	if err != nil {
		return false, fmt.Errorf("creating tmp dir: %w", err)
	}
	tmpfile := tmpdir + ".tar"
	if cleanup {
		defer cleanupRole(tmpdir, tmpfile)
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
	out, err := runClone(clone.String())
	if err != nil {
		return false, fmt.Errorf("cloning repo: %w\n%s", err, out)
	}

	sha, err := utils.Run("git rev-parse HEAD", tmpdir)
	if err != nil {
		return false, fmt.Errorf("getting commit hash: %w", err)
	}

	// check if the role is already installed
	installedCommit := entry.GetInstallInfo(rolesPath).InstallCommit
	if sha != "" && installedCommit != "" && sha == installedCommit {
		utils.Debug(name, "is already up to date")
		return false, nil
	}

	// create archive from the cloned source
	var archive strings.Builder
	archive.WriteString("git archive --prefix=")
	archive.WriteString(name)
	archive.WriteString("/ --output=")
	archive.WriteString(tmpfile)
	archive.WriteString(" ")
	archive.WriteString(entry.Version)
	out, err = utils.Run(archive.String(), tmpdir)
	if err != nil {
		return false, fmt.Errorf("archiving repo: %w\n%s", err, out)
	}

	// extract the archive into roles path
	out, err = utils.Run("tar -xf "+tmpfile, rolesPath)
	if err != nil {
		return false, fmt.Errorf("extracting archive: %w\n%s", err, out)
	}

	// write install info file
	outb, err := entry.GenerateInstallInfo(sha)
	if err != nil {
		return false, fmt.Errorf("generating install info: %w", err)
	}
	if err := os.WriteFile(path.Join(rolesPath, name, "meta", ".galaxy_install_info"), outb, 0o600); err != nil {
		return false, fmt.Errorf("writing install info: %w", err)
	}

	return true, nil
}

// bootstrapRoles creates the roles directory if it doesn't exist
func bootstrapRoles(rolesPath string) error {
	_, err := os.Stat(rolesPath)
	if err != nil && os.IsNotExist(err) {
		mkerr := os.Mkdir(rolesPath, 0o700)
		if mkerr != nil {
			return fmt.Errorf("creating roles path: %w", mkerr)
		}
	}
	return nil
}
