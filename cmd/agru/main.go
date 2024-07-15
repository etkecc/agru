package main

import (
	"flag"
	"fmt"

	"gitlab.com/etke.cc/int/agru/internal/parser"
	"gitlab.com/etke.cc/int/agru/internal/utils"
)

var (
	rolesPath              string
	requirementsPath       string
	updateRequirementsFile bool
	listInstalled          bool
	installMissing         bool
	verbose                bool
	cleanup                bool
)

func main() {
	flag.StringVar(&requirementsPath, "r", "requirements.yml", "ansible-galaxy requirements file")
	flag.StringVar(&rolesPath, "p", "roles/galaxy/", "path to install roles")
	flag.BoolVar(&listInstalled, "l", false, "list installed roles")
	flag.BoolVar(&installMissing, "i", true, "install missing roles")
	flag.BoolVar(&updateRequirementsFile, "u", false, "update requirements file if newer versions are available")
	flag.BoolVar(&cleanup, "c", true, "cleanup temporary files")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	utils.Verbose = verbose

	utils.Log(fmt.Sprintf("\033[1ma\033[0mnsible-\033[1mg\033[0malaxy \033[1mr\033[0mequirements.yml \033[1mu\033[0mpdater (list=%t update=%t cleanup=%t verbose=%t)", listInstalled, updateRequirementsFile, cleanup, verbose))
	utils.Log("parsing", requirementsPath)
	entries, installOnly := parser.ParseFile(requirementsPath)
	if updateRequirementsFile {
		utils.Log("updating", requirementsPath)
		parser.UpdateFile(entries, requirementsPath)
	}

	if listInstalled {
		installed := parser.GetInstalledRoles(rolesPath, parser.MergeFiles(entries, installOnly))
		for _, entry := range installed {
			fmt.Println("-", entry.GetName()+",", entry.GetInstallInfo(rolesPath).Version)
		}
	}

	if installMissing {
		utils.Log("installing/updating roles (if any)")
		parser.InstallMissingRoles(rolesPath, parser.MergeFiles(entries, installOnly), cleanup)
	}

	utils.Log("done")
}
