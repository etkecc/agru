package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/utils"
)

var (
	rolesPath              string
	requirementsPath       string
	updateRequirementsFile bool
	listInstalled          bool
	deleteInstalled        string
	installMissing         bool
	limit                  int
	verbose                bool
	cleanup                bool
)

func main() {
	flag.StringVar(&requirementsPath, "r", "requirements.yml", "ansible-galaxy requirements file")
	flag.StringVar(&rolesPath, "p", "roles/galaxy/", "path to install roles")
	flag.StringVar(&deleteInstalled, "d", "", "delete installed role, all other flags are ignored")
	flag.IntVar(&limit, "limit", 0, "limit the number of parallel downloads (affects roles installation only). 0 - no limit (default)")
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

	if deleteInstalled != "" {
		installed := parser.GetInstalledRoles(rolesPath, parser.MergeFiles(entries, installOnly))
		for _, entry := range installed {
			if entry.GetName() == deleteInstalled {
				utils.Log("deleting", entry.GetName())
				if err := os.RemoveAll(path.Join(rolesPath, entry.GetName())); err != nil {
					utils.Log("ERROR: cannot delete role:", err)
				}
				utils.Log("done")
				return
			}
		}
		utils.Log("role", deleteInstalled, "not found")
		return
	}

	if listInstalled {
		installed := parser.GetInstalledRoles(rolesPath, parser.MergeFiles(entries, installOnly))
		for _, entry := range installed {
			fmt.Println("-", entry.GetName()+",", entry.GetInstallInfo(rolesPath).Version)
		}
		return
	}

	if updateRequirementsFile {
		utils.Log("updating", requirementsPath)
		parser.UpdateFile(entries, requirementsPath)
	}

	if installMissing {
		utils.Log("installing/updating roles (if any)")
		parser.InstallMissingRoles(rolesPath, parser.MergeFiles(entries, installOnly), limit, cleanup)
	}

	utils.Log("done")
}
