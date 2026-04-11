package main

import (
	"flag"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/runner"
	"github.com/etkecc/agru/internal/tui"
	"github.com/etkecc/agru/internal/utils"
)

type config struct {
	rolesPath, requirementsPath, deleteInstalled                                  string
	limit                                                                         int
	listInstalled, installMissing, updateRequirementsFile, cleanup, verbose, keep bool
}

func main() {
	cfg := parseFlags()
	r := runner.New()
	p := parser.New(r)
	inst := installer.New(r, cfg.rolesPath, cfg.limit, cfg.cleanup)

	tuiCfg := tui.Config{
		RequirementsPath: cfg.requirementsPath,
		RolesPath:        cfg.rolesPath,
		DeleteName:       cfg.deleteInstalled,
		Limit:            cfg.limit,
		ListInstalled:    cfg.listInstalled,
		InstallMissing:   cfg.installMissing,
		UpdateFile:       cfg.updateRequirementsFile,
		Cleanup:          cfg.cleanup,
		Verbose:          cfg.verbose,
		Keep:             cfg.keep,
	}

	prog := tea.NewProgram(tui.New(tuiCfg, p, inst))
	if _, err := prog.Run(); err != nil {
		utils.Log("ERROR:", err)
		os.Exit(1)
	}
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
	flag.BoolVar(&cfg.keep, "k", false, "keep TUI open after completion until 'q'")
	flag.Parse()
	return cfg
}
