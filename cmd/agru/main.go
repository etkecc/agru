package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/runner"
	"github.com/etkecc/agru/internal/utils"
)

type config struct {
	rolesPath, requirementsPath, deleteInstalled                            string
	limit                                                                   int
	listInstalled, installMissing, updateRequirementsFile, cleanup, verbose bool
}

func main() {
	cfg := parseFlags()
	utils.Log(fmt.Sprintf("\033[1ma\033[0mnsible-\033[1mg\033[0malaxy \033[1mr\033[0mequirements.yml \033[1mu\033[0mpdater (list=%t update=%t cleanup=%t verbose=%t)", cfg.listInstalled, cfg.updateRequirementsFile, cfg.cleanup, cfg.verbose))

	r := runner.New(cfg.verbose)
	p := parser.New(r, cfg.verbose)
	inst := installer.New(r, cfg.rolesPath, cfg.limit, cfg.cleanup, cfg.verbose)

	utils.Log("parsing", cfg.requirementsPath)
	entries, installOnly, err := p.ParseFile(cfg.requirementsPath)
	if err != nil {
		utils.Log("ERROR: cannot parse requirements file:", err)
		os.Exit(1)
		return
	}

	if cfg.deleteInstalled != "" {
		deleteInstalledMode(inst, p, cfg, entries, installOnly)
		return
	}

	if cfg.listInstalled {
		listInstalledMode(inst, p, entries, installOnly)
		return
	}

	if cfg.updateRequirementsFile {
		utils.Log("updating", cfg.requirementsPath)
		if err := p.UpdateFile(entries, cfg.requirementsPath); err != nil {
			utils.Log("ERROR: cannot update requirements file:", err)
			os.Exit(1)
			return
		}
	}

	if cfg.installMissing {
		utils.Log("installing/updating roles (if any)")
		if err := inst.InstallMissing(p.MergeFiles(entries, installOnly)); err != nil {
			utils.Log("ERROR: cannot install roles:\n", err)
			os.Exit(1)
			return
		}
	}

	utils.Log("done")
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.requirementsPath, "r", "requirements.yml", "ansible-galaxy requirements file")
	flag.StringVar(&cfg.rolesPath, "p", "roles/galaxy/", "path to install roles")
	flag.StringVar(&cfg.deleteInstalled, "d", "", "delete installed role, all other flags are ignored")
	flag.IntVar(&cfg.limit, "limit", 0, "limit the number of parallel downloads (affects roles installation only). 0 - no limit (default)")
	flag.BoolVar(&cfg.listInstalled, "l", false, "list installed roles")
	flag.BoolVar(&cfg.installMissing, "i", true, "install missing roles")
	flag.BoolVar(&cfg.updateRequirementsFile, "u", false, "update requirements file if newer versions are available")
	flag.BoolVar(&cfg.cleanup, "c", true, "cleanup temporary files")
	flag.BoolVar(&cfg.verbose, "v", false, "verbose output")
	flag.Parse()
	return cfg
}

func deleteInstalledMode(inst *installer.Installer, p *parser.Parser, cfg config, entries, installOnly models.File) {
	installed := inst.GetInstalled(p.MergeFiles(entries, installOnly))
	for _, entry := range installed {
		if entry.GetName() == cfg.deleteInstalled {
			utils.Log("deleting", entry.GetName())
			if err := os.RemoveAll(path.Join(cfg.rolesPath, entry.GetName())); err != nil {
				utils.Log("ERROR: cannot delete role:", err)
				os.Exit(1)
				return
			}
			utils.Log("done")
			return
		}
	}
	utils.Log("role", cfg.deleteInstalled, "not found")
	os.Exit(1)
}

func listInstalledMode(inst *installer.Installer, p *parser.Parser, entries, installOnly models.File) {
	installed := inst.GetInstalled(p.MergeFiles(entries, installOnly))
	for _, entry := range installed {
		fmt.Println("-", entry.GetName()+",", entry.GetInstallInfo(inst.FS()).Version)
	}
}
