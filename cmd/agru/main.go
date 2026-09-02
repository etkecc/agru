package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
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

type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ";") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

// reorderArgs moves flags to the front so that shell-expanded positional args don't break flag parsing.
func reorderArgs(args []string) []string {
	var flags []string
	var pos []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// handle --flag=value
			if strings.Contains(arg, "=") {
				i++
				continue
			}
			name := strings.TrimLeft(arg, "-")
			takesValue := name == "r" || name == "p" || name == "d" || name == "limit"
			if takesValue && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i += 2
				continue
			}
			i++
		} else {
			pos = append(pos, arg)
			i++
		}
	}
	return append(flags, pos...)
}

func parseFlags() (config.Config, bool) {
	args := reorderArgs(os.Args[1:])
	fs := flag.NewFlagSet("agru", flag.ContinueOnError)
	var (
		cfg         config.Config
		showVersion bool
		reqPaths    stringSlice
	)
	fs.Var(&reqPaths, "r", "ansible-galaxy requirements file, wildcards supported (e.g. molecule/**/requirements.yml). Can be repeated")
	fs.StringVar(&cfg.RolesPath, "p", "roles/galaxy/", "path to install roles")
	fs.StringVar(&cfg.DeleteName, "d", "", "delete installed role, all other flags are ignored")
	fs.IntVar(&cfg.Limit, "limit", 0, "limit the number of parallel downloads (affects roles installation only). 0 - no limit (default)")
	fs.BoolVar(&cfg.ListInstalled, "l", false, "list installed roles")
	fs.BoolVar(&cfg.InstallMissing, "i", true, "install missing roles")
	fs.BoolVar(&cfg.UpdateFile, "u", false, "update requirements file if newer versions are available")
	fs.BoolVar(&cfg.Cleanup, "c", true, "cleanup temporary files")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "verbose output")
	fs.BoolVar(&cfg.Keep, "k", false, "keep TUI open after completion until 'q'")
	fs.BoolVar(&cfg.NoTUI, "no-tui", false, "force non-interactive logging output (no TUI)")
	fs.BoolVar(&showVersion, "v", false, "print version and exit")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		utils.Error(err)
		os.Exit(2)
	}

	paths := []string{}
	if len(reqPaths) > 0 {
		paths = append(paths, reqPaths...)
	}
	for _, a := range fs.Args() {
		if !strings.HasPrefix(a, "-") {
			paths = append(paths, a)
		}
	}
	if len(paths) == 0 {
		paths = []string{"requirements.yml"}
	}
	cfg.RequirementsPath = strings.Join(paths, ";")
	return cfg, showVersion
}
