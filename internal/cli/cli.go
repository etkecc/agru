// Package cli drives agru without the TUI: it consumes the same parser/installer
// progress channels synchronously and emits plain, colorless line logs, for when
// stdout is a file or a CI log instead of a terminal.
package cli

import (
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/config"
	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/utils"
)

// Run executes the flag-selected action against the same services the TUI uses, and
// returns non-zero-worthy errors so main can set the exit code. Mirrors handleParsed.
func Run(cfg config.Config, p *parser.Parser, inst *installer.Installer) error {
	entries, additional, extras, err := p.ParseFile(cfg.RequirementsPath)
	if err != nil {
		utils.Error(err)
		return err
	}
	merged := p.MergeFiles(entries, additional)

	switch {
	case cfg.ListInstalled:
		return runList(inst, merged)
	case cfg.DeleteName != "":
		return runDelete(cfg, merged)
	case cfg.UpdateFile:
		// on -u -i we stop if the update failed. the TUI installs anyway and ships
		// versions that never reached requirements.yml, which is how you get an
		// unreproducible box; we refuse that here.
		if err := runUpdate(cfg, p, entries, extras); err != nil {
			return err
		}
		if !cfg.InstallMissing {
			return nil
		}
		// UpdateFile rewrote entries in place, so re-merge to hand the installer the bumped versions.
		return runInstall(cfg, inst, p.MergeFiles(entries, additional))
	case cfg.InstallMissing:
		return runInstall(cfg, inst, merged)
	}
	return nil
}

// runList prints one "name version" line per installed role; a role whose install
// info won't parse goes to stderr and the rest keep listing.
func runList(inst *installer.Installer, merged models.File) error {
	installed := inst.GetInstalled(merged)
	for _, e := range installed {
		info, err := e.GetInstallInfo(inst.FS())
		if err != nil {
			utils.Error(e.GetName(), err)
			continue
		}
		utils.Log(e.GetName(), info.Version)
	}
	return nil
}

// runDelete removes one role's directory. success is silent, matching the TUI's silent quit.
func runDelete(cfg config.Config, merged models.File) error {
	for _, entry := range merged {
		if entry.GetName() != cfg.DeleteName {
			continue
		}
		if err := os.RemoveAll(path.Join(cfg.RolesPath, entry.GetName())); err != nil {
			utils.Error(err)
			return err
		}
		return nil
	}
	err := fmt.Errorf("role %q not found", cfg.DeleteName)
	utils.Error(err)
	return err
}

// runUpdate drains the version-check channel to stdout and returns the write error the
// TUI drops: a failed requirements.yml write surfaces here as a non-zero exit.
func runUpdate(cfg config.Config, p *parser.Parser, entries models.File, extras map[string]yaml.Node) error {
	ch := make(chan parser.CheckProgress, 64)
	errCh := make(chan error, 1)
	go func() { errCh <- p.UpdateFile(entries, extras, cfg.RequirementsPath, ch) }()
	var streamed bool
	for pr := range ch {
		switch {
		case pr.Err != nil:
			streamed = true
			utils.Error(pr.Name, pr.Err)
		case pr.NewVer != "":
			utils.Log(pr.Name, pr.OldVer, "->", pr.NewVer)
		default:
			utils.Debug(cfg.Verbose, pr.Name, pr.OldVer, "(up to date)")
		}
	}
	err := <-errCh
	if err != nil && !streamed {
		utils.Error(err) // the requirements.yml write failure, which never rides the channel
	}
	return err
}

// runInstall drains the install channel to stdout; per-role errors already print here,
// so the returned aggregate only drives the exit code.
func runInstall(cfg config.Config, inst *installer.Installer, merged models.File) error {
	ch := make(chan installer.Progress, 64)
	errCh := make(chan error, 1)
	go func() { errCh <- inst.InstallMissing(merged, ch) }()
	var streamed bool
	for pr := range ch {
		switch pr.Status {
		case "active":
			utils.Debug(cfg.Verbose, "installing", pr.Name, pr.Version)
		case "skipped":
			utils.Debug(cfg.Verbose, pr.Name, "(up to date)")
		case "done":
			line := pr.Version
			if pr.OldVersion != "" && pr.OldVersion != pr.Version {
				line = pr.OldVersion + " -> " + pr.Version
			}
			utils.Log(pr.Name, line)
		case "error":
			streamed = true
			utils.Error(pr.Name, pr.Err)
		}
		if cfg.Verbose && pr.Log != "" {
			utils.Log(pr.Log) // already carries a [name] prefix from the installer
		}
	}
	err := <-errCh
	if err != nil && !streamed {
		utils.Error(err) // a top-level failure like bootstrapRoles, which never rides the channel
	}
	return err
}
