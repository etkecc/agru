package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/runner"
	"github.com/etkecc/agru/internal/tui"
	"github.com/etkecc/agru/internal/utils"
)

// version is set by goreleaser via -X main.version={{.Version}} at release build time.
var version = ""

type config struct {
	rolesPath, requirementsPath, deleteInstalled                                           string
	limit                                                                                  int
	listInstalled, installMissing, updateRequirementsFile, cleanup, verbose, keep, version bool
}

func getVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 7 {
				rev = rev[:7]
			}
		case "vcs.modified":
			if s.Value == "true" {
				modified = "-dirty"
			}
		}
	}
	if rev != "" {
		return rev + modified
	}
	return "dev"
}

func main() {
	cfg := parseFlags()
	if cfg.version {
		fmt.Println(getVersion())
		return
	}
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
	flag.BoolVar(&cfg.verbose, "verbose", false, "verbose output")
	flag.BoolVar(&cfg.keep, "k", false, "keep TUI open after completion until 'q'")
	flag.BoolVar(&cfg.version, "v", false, "print version and exit")
	flag.BoolVar(&cfg.version, "version", false, "print version and exit")
	flag.Parse()
	return cfg
}
