package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type RequirementsFile []RequirementsEntry
type RequirementsEntry struct {
	Src     string `yaml:"src"`
	Version string `yaml:"version"`
	Name    string `yaml:"name,omitempty"`
}

var requirementsPath string
var ignoredVersions = map[string]bool{
	"main":   true,
	"master": true,
}

func main() {
	flag.StringVar(&requirementsPath, "r", "requirements.yml", "ansible-galaxy requirements file")
	flag.Parse()

	log.Println("updating requirements.yml...")
	updateRequirements(requirementsPath)
}

func updateRequirements(path string) {
	entries := parseRequirements(path)

	var wg sync.WaitGroup
	wg.Add(len(entries))
	for i, entry := range entries {
		go func(i int, entry RequirementsEntry, wg *sync.WaitGroup) {
			newVersion := getNewVersion(entry.Src, entry.Version)
			if newVersion != "" {
				log.Println(entry.Src, entry.Version, "->", newVersion)
				entry.Version = newVersion
				entries[i] = entry
			}
			wg.Done()
		}(i, entry, &wg)
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

func parseRequirements(path string) RequirementsFile {
	fileb, err := os.ReadFile(path)
	if err != nil {
		log.Println("ERROR: ", err)
		return RequirementsFile{}
	}
	var req RequirementsFile
	if err := yaml.Unmarshal(fileb, &req); err != nil {
		log.Println("ERROR: ", err)
	}

	return req
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
	tags, err := execute("git ls-remote -tq --sort=-version:refname " + repo)
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

func execute(command string) (string, error) {
	slice := strings.Split(command, " ")
	out, err := exec.Command(slice[0], slice[1:]...).CombinedOutput()
	if out == nil {
		return "", err
	}

	return strings.TrimSuffix(string(out), "\n"), err
}
