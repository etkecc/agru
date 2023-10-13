package main

import (
	"flag"
	"log"
	"os/exec"
	"strings"
)

var (
	rolesPath              string
	requirementsPath       string
	updateRequirementsFile bool
	verbose                bool
	cleanup                bool
)

func main() {
	flag.StringVar(&requirementsPath, "r", "requirements.yml", "ansible-galaxy requirements file")
	flag.StringVar(&rolesPath, "p", "roles/galaxy/", "path to install roles")
	flag.BoolVar(&updateRequirementsFile, "u", false, "update requirements file if newer versions are available")
	flag.BoolVar(&cleanup, "c", true, "cleanup temporary files")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	log.Println("parsing", requirementsPath)
	entries, installOnly := parseRequirements(requirementsPath)
	if updateRequirementsFile {
		log.Println("updating", requirementsPath)
		updateRequirements(entries)
	}

	log.Println("installing/updating roles (if any)")
	installMissingRoles(mergeRequirementsEntries(entries, installOnly))

	log.Println("done")
}

func execute(command, dir string) (string, error) {
	slice := strings.Split(command, " ")
	cmd := exec.Command(slice[0], slice[1:]...) //nolint:gosec // that's intended
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if verbose {
		log.Println("DEBUG: execute")
		log.Println("       command:", command)
		log.Println("       chdir:", dir)
		if out != nil {
			log.Println("       output:", strings.TrimSuffix(string(out), "\n"))
		}
	}
	if out == nil {
		return "", err
	}

	return strings.TrimSuffix(string(out), "\n"), err
}
