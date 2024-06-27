package parser

import (
	"os"
	"path"
	"strings"
	"sync"

	"gitlab.com/etke.cc/int/agru/internal/models"
	"gitlab.com/etke.cc/int/agru/internal/utils"
)

var ignoredVersions = map[string]bool{
	"main":   true,
	"master": true,
}

// InstallMissingRoles writes all roles to the target roles dir if role doesn't exist or has different version
func InstallMissingRoles(rolesPath string, entries models.File, cleanup bool) {
	_, err := os.Stat(rolesPath)
	if err != nil && os.IsNotExist(err) {
		mkerr := os.Mkdir(rolesPath, 0o700)
		if mkerr != nil {
			utils.Log("ERROR: cannot create roles path:", mkerr)
		}
	}
	var wg sync.WaitGroup
	wg.Add(len(entries))
	for _, entry := range entries {
		go func(entry *models.Entry, wg *sync.WaitGroup) {
			if !entry.IsInstalled(rolesPath) {
				installRole(rolesPath, entry, cleanup)
			}
			wg.Done()
		}(entry, &wg)
	}
	wg.Wait()
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
		utils.Log("ERROR:", err)
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

// installRole writes specific role version to the target roles dir
func installRole(rolesPath string, entry *models.Entry, cleanup bool) {
	name := entry.GetName()
	utils.Log("installing", name, entry.Version)

	repo := strings.Replace(entry.Src, "git+", "", 1)
	tmpdir, err := os.MkdirTemp("", "agru-"+name+"-*")
	if err != nil {
		utils.Log("ERROR: cannot create tmp dir:", err)
		return
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
	out, err := utils.Run(clone.String(), "")
	if err != nil {
		utils.Log("ERROR: cannot clone repo:", err)
		utils.Log(out)
		return
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
		return
	}

	// extract the archive into roles path
	out, err = utils.Run("tar -xf "+tmpfile, rolesPath)
	if err != nil {
		utils.Log("ERROR: cannot extract archive:", err)
		utils.Log(out)
		return
	}

	// write install info file
	outb, err := entry.GenerateInstallInfo()
	if err != nil {
		utils.Log("ERROR: cannot generate install info:", err)
		return
	}
	if err := os.WriteFile(path.Join(rolesPath, name, "meta", ".galaxy_install_info"), outb, 0o600); err != nil {
		utils.Log("ERROR: cannot write install info:", err)
		return
	}
}
