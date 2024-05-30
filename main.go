package main

import (
	"flag"
	"fmt"
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

	log(fmt.Sprintf("\033[1ma\033[0mnsible-\033[1mg\033[0malaxy \033[1mr\033[0mequirements.yml \033[1mu\033[0mpdater (update=%t cleanup=%t verbose=%t)", updateRequirementsFile, cleanup, verbose))
	log("parsing", requirementsPath)
	entries, installOnly := parseRequirements(requirementsPath)
	if updateRequirementsFile {
		log("updating", requirementsPath)
		updateRequirements(entries)
	}

	log("installing/updating roles (if any)")
	installMissingRoles(mergeRequirementsEntries(entries, installOnly))

	log("done")
}

func execute(command, dir string) (string, error) {
	slice := strings.Split(command, " ")
	cmd := exec.Command(slice[0], slice[1:]...) //nolint:gosec // that's intended
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	debug("execute")
	debug("    command:", command)
	debug("    chdir:", dir)
	if out != nil {
		debug("    output:", strings.TrimSuffix(string(out), "\n"))
	}
	if out == nil {
		return "", err
	}

	return strings.TrimSuffix(string(out), "\n"), err
}

func log(v ...any) {
	v = append([]any{"[a.g.r.u]"}, v...)
	fmt.Println(v...)
}

func debug(v ...any) {
	if verbose {
		log(v...)
	}
}
