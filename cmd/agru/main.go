package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/etkecc/go-kit"

	"github.com/etkecc/agru/internal/cli"
	"github.com/etkecc/agru/internal/config"
	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/pathfinder"
	"github.com/etkecc/agru/internal/runner"
	"github.com/etkecc/agru/internal/utils"
)

var version = sync.OnceValue(func() string { return kit.Version("", "") })()

func main() {
	cfg, showVersion := parseFlags()
	if showVersion {
		fmt.Println(version)
		return
	}
	// version and help need no paths, so resolution happens once we know we are doing real work
	if err := pathfinder.ResolveInstallPaths(&cfg); err != nil {
		utils.Error(err)
		os.Exit(2)
	}
	utils.Debug(cfg.Verbose, "roles path:", cfg.RolesPath)
	utils.Debug(cfg.Verbose, "collections path:", cfg.CollectionsPath)
	r := runner.New()
	p := parser.New(r)
	inst := installer.New(r, cfg.RolesPath, cfg.CollectionsPath, cfg.Limit, cfg.Cleanup)

	if err := cli.Run(&cfg, p, inst); err != nil {
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
			takesValue := name == "r" || name == "p" || name == "cp" || name == "d" || name == "limit"
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
	fs.StringVar(&cfg.RolesPath, "p", "", "path to install roles (default: $ANSIBLE_ROLES_PATH, then $ANSIBLE_HOME/roles, then ~/.ansible/roles)")
	fs.StringVar(&cfg.CollectionsPath, "cp", "", "path to install collections (default: $ANSIBLE_COLLECTIONS_PATH, then $ANSIBLE_COLLECTIONS_PATHS, then $ANSIBLE_HOME/collections, then ~/.ansible/collections)")
	fs.StringVar(&cfg.DeleteName, "d", "", "delete installed role or collection, all other flags are ignored")
	fs.IntVar(&cfg.Limit, "limit", 0, "limit the number of parallel downloads (affects roles installation only). 0 - no limit (default)")
	fs.BoolVar(&cfg.ListInstalled, "l", false, "list installed roles and collections")
	fs.BoolVar(&cfg.InstallMissing, "i", true, "install missing roles and collections")
	fs.BoolVar(&cfg.UpdateFile, "u", false, "update requirements file if newer versions are available")
	fs.BoolVar(&cfg.Cleanup, "c", true, "cleanup temporary files")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "verbose output")
	fs.BoolVar(&cfg.NoTUI, "no-tui", false, "deprecated, interactive mode was removed")
	fs.BoolVar(&showVersion, "v", false, "print version and exit")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		utils.Error(err)
		os.Exit(2)
	}
	paths := []string{}
	if len(reqPaths) > 0 {
		for _, p := range reqPaths {
			if _, err := os.Stat(p); err == nil {
				paths = append(paths, p)
			}
		}
	}
	for _, a := range fs.Args() {
		if !strings.HasPrefix(a, "-") {
			if _, err := os.Stat(a); err == nil {
				paths = append(paths, a)
			}
		}
	}
	if len(paths) == 0 {
		paths = []string{"requirements.yml"}
	}
	cfg.RequirementsPath = strings.Join(paths, ";")

	return cfg, showVersion
}
