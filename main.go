package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// RequirementsFile structure
type RequirementsFile []RequirementsEntry

// RequirementsEntry is requirements.yml's entry structure
type RequirementsEntry struct {
	Src     string `yaml:"src,omitempty"`
	Version string `yaml:"version,omitempty"`
	Name    string `yaml:"name,omitempty"`
	Include string `yaml:"include,omitempty"`
}

// GalaxyInstallInfo is meta/.galaxy_install_info struct
type GalaxyInstallInfo struct {
	InstallDate string `yaml:"install_date"`
	Version     string `yaml:"version"`
}

var (
	rolesPath        string
	requirementsPath string
	ignoredVersions  = map[string]bool{
		"main":   true,
		"master": true,
	}
)

func main() {
	flag.StringVar(&requirementsPath, "r", "requirements.yml", "ansible-galaxy requirements file")
	flag.StringVar(&rolesPath, "p", "roles/galaxy/", "path to install roles")
	flag.Parse()

	log.Println("updating requirements.yml...")
	updateRequirements(requirementsPath)
}

func updateRequirements(path string) {
	entries, installOnly := parseRequirements(path)

	var wg sync.WaitGroup
	wg.Add(len(entries) + len(installOnly))
	for i, entry := range entries {
		go func(i int, entry RequirementsEntry, wg *sync.WaitGroup) {
			newVersion := getNewVersion(entry.Src, entry.Version)
			if newVersion != "" {
				log.Println(entry.Src, entry.Version, "->", newVersion)
				entry.Version = newVersion
				installRole(entry)
				entries[i] = entry
			}
			if !isInstalled(entry) {
				installRole(entry)
			}
			wg.Done()
		}(i, entry, &wg)
	}
	for _, entry := range installOnly {
		go func(entry RequirementsEntry, wg *sync.WaitGroup) {
			if !isInstalled(entry) {
				installRole(entry)
			}
			wg.Done()
		}(entry, &wg)
	}
	wg.Wait()

	outb, err := yaml.Marshal(entries)
	if err != nil {
		log.Println("ERROR: ", err)
		return
	}
	if err := os.WriteFile(path, outb, 0600); err != nil {
		log.Println("ERROR: ", err)
	}
}

// parseRequirements parses requirements.yml file and tries to update it
// if it founds any includes within that file, they will be returned as second return value
func parseRequirements(path string) (RequirementsFile, RequirementsFile) {
	fileb, err := os.ReadFile(path)
	if err != nil {
		log.Println("ERROR: ", err)
		return RequirementsFile{}, RequirementsFile{}
	}
	var req RequirementsFile
	if err := yaml.Unmarshal(fileb, &req); err != nil {
		log.Println("ERROR: ", err)
	}

	return req, parseAdditionalRequirements(req)
}

func parseAdditionalRequirements(req RequirementsFile) RequirementsFile {
	additional := make([]RequirementsEntry, 0)
	for _, entry := range req {
		if entry.Include != "" {
			// no recursive iteration over deeper levels, because it's not used anywhere
			additionalLvl1, additionalLvl2 := parseRequirements(entry.Include)
			additional = append(additional, additionalLvl1...)
			additional = append(additional, additionalLvl2...)
		}
	}

	return additional
}

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

func execute(command string, dir string) (string, error) {
	slice := strings.Split(command, " ")
	cmd := exec.Command(slice[0], slice[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if out == nil {
		return "", err
	}

	return strings.TrimSuffix(string(out), "\n"), err
}

// getRoleName returns either role name or repo name from url
func getRoleName(entry RequirementsEntry) string {
	if entry.Name != "" {
		return entry.Name
	}
	return strings.TrimSuffix(path.Base(entry.Src), ".git")
}

func generateInstallInfo(version string) ([]byte, error) {
	info := GalaxyInstallInfo{
		InstallDate: time.Now().Format("Mon 02 Jan 2006 03:04:05 PM "), // the trailing space is done by ansible-galaxy
		Version:     version,
	}
	return yaml.Marshal(info)
}

func isInstalled(entry RequirementsEntry) bool {
	_, err := os.Stat(path.Join(rolesPath, getRoleName(entry)))
	if err == nil {
		return true
	}
	return os.IsExist(err)
}

func installRole(entry RequirementsEntry) {
	name := getRoleName(entry)
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
	clone.WriteString("git clone -q --depth 1 -b ")
	clone.WriteString(entry.Version)
	clone.WriteString(" ")
	clone.WriteString(repo)
	clone.WriteString(" ")
	clone.WriteString(tmpdir)
	_, err = execute(clone.String(), "")
	if err != nil {
		log.Println("ERROR: cannot clone repo:", err)
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
	_, err = execute(archive.String(), tmpdir)
	if err != nil {
		log.Println("ERROR: cannot archive repo:", err)
		return
	}

	// extract the archive into roles path
	out, err := execute("tar -xf "+tmpfile, rolesPath)
	if err != nil {
		log.Println("ERROR: cannot extract archive:", err)
		log.Println(out)
		return
	}

	// write install info file
	outb, err := generateInstallInfo(entry.Version)
	if err != nil {
		log.Println("ERROR: cannot generate install info:", err)
		return
	}
	if err := os.WriteFile(path.Join(rolesPath, name, "meta", ".galaxy_install_info"), outb, 0600); err != nil {
		log.Println("ERROR: cannot write install info:", err)
		return
	}
}
