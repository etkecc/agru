package parser

import (
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
func InstallMissingRoles(rolesPath string, entries models.File, limit int, cleanup bool) {
	bootstrapRoles(rolesPath)
	rolesLen := entries.RolesLen()
	// if limit is 0, then no limit
	if limit == 0 {
		limit = rolesLen
	}
	bar := utils.NewProgressbar(rolesLen, "installing roles")
	wp := workpool.New(limit)
	changes := models.UpdatedItems{}
	for _, entry := range entries {
		if entry.Include != "" { // skip entries with include directive
			continue
		}
		wp.Do(func() {
			defer bar.Add(1) //nolint:errcheck // don't care about error here

			if !entry.IsInstalled(rolesPath) {
				if !ignoredVersions[entry.Version] {
					changes = changes.Add(entry.GetName(), entry.GetInstallInfo(rolesPath).Version, entry.Version)
				}
				if installRole(rolesPath, entry, cleanup) {
					bar.AddDetail(fmt.Sprintf("installed %s@%s", entry.GetName(), entry.Version)) //nolint:errcheck // don't care about error here
				}
			}
		})
	}
	wp.Run()

	if len(changes) > 0 {
		utils.Log(changes.String("roles updated:\n"))
	}
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
func getNewVersion(src, version string) string {
	if ignoredVersions[version] {
		return ""
	}

	// not a git repo
	if !strings.Contains(src, "git") {
		return ""
	}

	repo := strings.Replace(src, "git+https", "https", 1)
	tags, err := utils.Run("git ls-remote -tq --sort=-version:refname "+repo, "")
	if err != nil {
		utils.Log("ERROR: git ls-remote", repo, err)
		return ""
	}
	if tags == "" {
		return ""
	}

	lastline := strings.Split(tags, "\n")[0]
	tagidx := strings.Index(lastline, "refs/tags/")
	if tagidx == -1 {
		utils.Log("ERROR: lastline:", lastline)
		return ""
	}
	last := strings.Replace(lastline[tagidx:], "refs/tags/", "", 1)
	last = strings.Replace(last, "^{}", "", 1) // NOTE: very weird case with some github repos, didn't find out why it does that
	if last != version {
		return last
	}

	return ""
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
		utils.Log("ERROR: cannot clone repo, retrying in", delay.String(), out)
		time.Sleep(delay)
		return runClone(cmd, attempt+1)
	}

	utils.Log("ERROR: cannot clone repo:", err)
	utils.Log(out)
	return out, err
}

// installRole writes specific role version to the target roles dir
func installRole(rolesPath string, entry *models.Entry, cleanup bool) bool {
	name := entry.GetName()

	repo := strings.Replace(entry.Src, "git+", "", 1)
	tmpdir, err := os.MkdirTemp("", "agru-"+name+"-*")
	if err != nil {
		utils.Log("ERROR: cannot create tmp dir:", err)
		return false
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
		utils.Log("ERROR: cannot clone repo:", err)
		utils.Log(out)
		return false
	}

	sha, err := utils.Run("git rev-parse HEAD", tmpdir)
	if err != nil {
		utils.Log("ERROR: cannot get commit hash:", err)
		return false
	}

	// check if the role is already installed
	installedCommit := entry.GetInstallInfo(rolesPath).InstallCommit
	if sha != "" && installedCommit != "" && sha == entry.GetInstallInfo(rolesPath).InstallCommit {
		utils.Debug(name, "is already up to date")
		return false
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
		utils.Log("ERROR: cannot archive repo:", err)
		utils.Log(out)
		return false
	}

	// extract the archive into roles path
	out, err = utils.Run("tar -xf "+tmpfile, rolesPath)
	if err != nil {
		utils.Log("ERROR: cannot extract archive:", err)
		utils.Log(out)
		return false
	}

	// write install info file
	outb, err := entry.GenerateInstallInfo(sha)
	if err != nil {
		utils.Log("ERROR: cannot generate install info:", err)
		return false
	}
	if err := os.WriteFile(path.Join(rolesPath, name, "meta", ".galaxy_install_info"), outb, 0o600); err != nil {
		utils.Log("ERROR: cannot write install info:", err)
		return false
	}

	return true
}

// bootstrapRoles creates the roles directory if it doesn't exist
func bootstrapRoles(rolesPath string) {
	_, err := os.Stat(rolesPath)
	if err != nil && os.IsNotExist(err) {
		mkerr := os.Mkdir(rolesPath, 0o700)
		if mkerr != nil {
			utils.Log("ERROR: cannot create roles path:", mkerr)
		}
	}
}
