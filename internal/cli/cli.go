// Package cli drives agru without the TUI, emitting plain colorless line logs for a file or CI log.
package cli

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/etkecc/agru/internal/config"
	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/utils"
)

// Run executes the flag-selected action against the same services the TUI uses. Mirrors handleParsed.
func Run(cfg config.Config, p *parser.Parser, inst *installer.Installer) error {
	files, err := p.ParsePattern(cfg.RequirementsPath)
	if err != nil {
		utils.Error(err)
		return err
	}
	merged := p.MergeAll(files)

	switch {
	case cfg.ListInstalled:
		return runList(inst, merged)
	case cfg.DeleteName != "":
		return runDelete(cfg, merged)
	case cfg.UpdateFile:
		// on -u -i we stop if the update failed, unlike the TUI, which installs unreproducible versions anyway.
		if err := runUpdate(cfg, p, files); err != nil {
			return err
		}
		if !cfg.InstallMissing {
			return nil
		}
		// UpdateAll rewrote entries in place, so re-merge to hand the installer the bumped versions.
		return runInstall(cfg, inst, p.MergeAll(files))
	case cfg.InstallMissing:
		return runInstall(cfg, inst, merged)
	}
	return nil
}

// runList prints one "name version" line per installed role; a role whose install info won't parse goes to stderr.
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

// runUpdate drains the version-check channel to stdout and aggregates per-file errors into the returned error.
func runUpdate(cfg config.Config, p *parser.Parser, files []parser.RequirementsFile) error {
	ch := make(chan parser.CheckProgress, 64)
	errCh := make(chan error, 1)
	go func() { errCh <- updateErrors(files, p.UpdateAll(files, ch)) }()
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
	// multi-file: a write failure must surface even when another file streamed check errors
	if err != nil && (len(files) > 1 || !streamed) {
		utils.Error(err)
	}
	return err
}

// updateErrors joins UpdateAll's per-file errors; a path prefix goes on only when several files matched.
func updateErrors(files []parser.RequirementsFile, errs []error) error {
	var agg []string
	prefix := len(files) > 1 // single file: keep the original unprefixed error wording
	for i, err := range errs {
		if err == nil {
			continue
		}
		if prefix {
			agg = append(agg, files[i].Path+": "+err.Error())
		} else {
			agg = append(agg, err.Error())
		}
	}
	if len(agg) > 0 {
		return fmt.Errorf("%s", strings.Join(agg, "\n"))
	}
	return nil
}

// runInstall drains the install channel to stdout; the returned aggregate only drives the exit code.
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
