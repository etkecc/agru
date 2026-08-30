package main

import (
	"flag"
	"fmt"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/etkecc/go-kit"

	"github.com/etkecc/agru/internal/cli"
	"github.com/etkecc/agru/internal/config"
	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/runner"
	"github.com/etkecc/agru/internal/tui"
	"github.com/etkecc/agru/internal/utils"
)

var version = sync.OnceValue(func() string { return kit.Version("", "") })()

func main() {
	cfg, showVersion := parseFlags()
	if showVersion {
		fmt.Println(version)
		return
	}
	r := runner.New()
	p := parser.New(r)
	inst := installer.New(r, cfg.RolesPath, cfg.Limit, cfg.Cleanup)

	// No usable terminal (piped, redirected, CI) or an explicit opt-out: drop the TUI
	// and log plain text instead, or bubbletea paints escape codes into the file.
	if cfg.NoTUI || !term.IsTerminal(os.Stdout.Fd()) {
		if err := cli.Run(cfg, p, inst); err != nil {
			os.Exit(1)
		}
		return
	}

	prog := tea.NewProgram(tui.New(cfg, p, inst))
	if _, err := prog.Run(); err != nil {
		utils.Error(err)
		os.Exit(1)
	}
}

func parseFlags() (config.Config, bool) {
	var (
		cfg         config.Config
		showVersion bool
	)
	flag.StringVar(&cfg.RequirementsPath, "r", "requirements.yml", "ansible-galaxy requirements file, wildcards supported (e.g. molecule/**/requirements.yml)")
	flag.StringVar(&cfg.RolesPath, "p", "roles/galaxy/", "path to install roles")
	flag.StringVar(&cfg.DeleteName, "d", "", "delete installed role, all other flags are ignored")
	flag.IntVar(&cfg.Limit, "limit", 0, "limit the number of parallel downloads (affects roles installation only). 0 - no limit (default)")
	flag.BoolVar(&cfg.ListInstalled, "l", false, "list installed roles")
	flag.BoolVar(&cfg.InstallMissing, "i", true, "install missing roles")
	flag.BoolVar(&cfg.UpdateFile, "u", false, "update requirements file if newer versions are available")
	flag.BoolVar(&cfg.Cleanup, "c", true, "cleanup temporary files")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "verbose output")
	flag.BoolVar(&cfg.Keep, "k", false, "keep TUI open after completion until 'q'")
	flag.BoolVar(&cfg.NoTUI, "no-tui", false, "force non-interactive logging output (no TUI)")
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	return cfg, showVersion
}
