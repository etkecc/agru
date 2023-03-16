package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

type RequirementsFile []RequirementsEntry
type RequirementsEntry struct {
	Src     string `yaml:"src"`
	Version string `yaml:"version"`
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
	for i, entry := range entries {
		newVersion := updateRequirementEntry(entry)
		if newVersion != "" {
			log.Println(entry.Src, entry.Version, "->", newVersion)
			entries[i].Version = newVersion
		}
	}
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

func updateRequirementEntry(entry RequirementsEntry) string {
	if ignoredVersions[entry.Version] {
		return ""
	}

	// not a git repo
	if strings.Index(entry.Src, "https") == -1 && strings.Index(entry.Src, "git") == -1 {
		return ""
	}

	repo := strings.Replace(entry.Src, "git+https", "https", 1)
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
	if last != entry.Version {
		return last
	}

	return ""
}

func execute(command string) (string, error) {
	slice := strings.Split(command, " ")
	var cmd *exec.Cmd
	if len(slice) == 1 {
		cmd = exec.Command(slice[0])
	} else {
		cmd = exec.Command(slice[0], slice[1:]...)
	}

	out, err := cmd.CombinedOutput()
	if out == nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), err
}
