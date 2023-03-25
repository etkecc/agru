package main

import (
	"log"
	"os"
	"path"
	"strings"
	"sync"
)

var ignoredVersions = map[string]bool{
	"main":   true,
	"master": true,
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
	tags, err := execute("git ls-remote -tq --sort=-version:refname "+repo, "")
	if err != nil {
		log.Println("ERROR: ", err)
		return ""
	}
	if tags == "" {
		return ""
	}

	lastline := strings.Split(tags, "\n")[0]
	tagidx := strings.Index(lastline, "refs/tags/")
	if tagidx == -1 {
		log.Println("ERROR: lastline: ", lastline)
		return ""
	}
	last := strings.Replace(lastline[tagidx:], "refs/tags/", "", 1)
	last = strings.Replace(last, "^{}", "", 1) // NOTE: very weird case with some github repos, didn't find out why it does that
	if last != version {
		return last
	}

	return ""
}

// installRole writes specific role version to the target roles dir
func installRole(entry RequirementsEntry) {
	name := entry.GetName()
	log.Println("Installing", name, entry.Version)

	repo := strings.Replace(entry.Src, "git+", "", 1)
	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		log.Println("ERROR: cannot create tmp dir:", err)
		return
	}
	tmpfile := tmpdir + ".tar"

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
	out, err := execute(clone.String(), "")
	if err != nil {
		log.Println("ERROR: cannot clone repo:", err)
		log.Println(out)
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
	out, err = execute(archive.String(), tmpdir)
	if err != nil {
		log.Println("ERROR: cannot archive repo:", err)
		log.Println(out)
		return
	}

	// extract the archive into roles path
	out, err = execute("tar -xf "+tmpfile, rolesPath)
	if err != nil {
		log.Println("ERROR: cannot extract archive:", err)
		log.Println(out)
		return
	}

	// write install info file
	outb, err := entry.GenerateInstallInfo()
	if err != nil {
		log.Println("ERROR: cannot generate install info:", err)
		return
	}
	if err := os.WriteFile(path.Join(rolesPath, name, "meta", ".galaxy_install_info"), outb, 0o600); err != nil {
		log.Println("ERROR: cannot write install info:", err)
		return
	}
}

// installMissingRoles writes all roles to the target roles dir if role doesn't exist or has different version
func installMissingRoles(entries RequirementsFile) {
	var wg sync.WaitGroup
	wg.Add(len(entries))
	for _, entry := range entries {
		go func(entry RequirementsEntry, wg *sync.WaitGroup) {
			if !entry.IsInstalled() {
				installRole(entry)
			}
			wg.Done()
		}(entry, &wg)
	}
	wg.Wait()
}
