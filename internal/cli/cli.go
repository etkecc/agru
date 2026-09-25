// Package cli drives agru, emitting plain colorless line logs for a file or CI log.
package cli

import (
	"fmt"
	"path"
	"strings"

	"github.com/etkecc/agru/internal/config"
	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/parser"
	"github.com/etkecc/agru/internal/utils"
)

// Run executes the flag-selected action: parse requirements, then list, delete, update, or install.
func Run(cfg *config.Config, p *parser.Parser, inst *installer.Installer) error {
	files, err := p.ParsePattern(cfg.RequirementsPath)
	if err != nil {
		utils.Error(err)
		return err
	}
	merged := p.MergeAll(files)
	mergedColls := p.MergeAllCollections(files)

	switch {
	case cfg.ListInstalled:
		return runList(inst, merged, mergedColls)
	case cfg.DeleteName != "":
		return runDelete(cfg, merged, mergedColls)
	case cfg.UpdateFile:
		// on -u -i we stop if the update failed, which would otherwise install unreproducible versions.
		if err := runUpdate(cfg, p, files); err != nil {
			return err
		}
		if !cfg.InstallMissing {
			return nil
		}
		// UpdateAll rewrote entries in place, so re-merge to hand the installer the bumped versions.
		return runInstall(cfg, inst, p.MergeAll(files), p.MergeAllCollections(files))
	case cfg.InstallMissing:
		return runInstall(cfg, inst, merged, mergedColls)
	}
	return nil
}

// runList prints one "name version" line per installed role and collection.
func runList(inst *installer.Installer, merged models.File, mergedColls models.Collections) error {
	installed := inst.GetInstalled(merged)
	for _, e := range installed {
		info, err := e.GetInstallInfo(inst.FS())
		if err != nil {
			utils.Error(e.GetName(), err)
			continue
		}
		utils.Log(e.GetName(), info.Version)
	}
	// List installed collections
	installedColls := inst.GetInstalledCollections(mergedColls)
	for _, c := range installedColls {
		utils.Log(c.GetFQCN(), c.GetInstalledVersion(inst.CollFS()))
	}
	return nil
}

// runDelete removes one role or collection directory by name.
func runDelete(cfg *config.Config, merged models.File, mergedColls models.Collections) error {
	// Roles first (win on collision)
	for _, entry := range merged {
		if entry.GetName() != cfg.DeleteName {
			continue
		}
		if !models.IsValidRoleName(entry.GetName()) {
			err := fmt.Errorf("invalid role name %q", entry.GetName())
			utils.Error(err)
			return err
		}
		if err := installer.RemoveRoleDir(path.Join(cfg.RolesPath, entry.GetName())); err != nil {
			utils.Error(err)
			return err
		}
		return nil
	}
	// Then collections
	for _, c := range mergedColls {
		if c.GetFQCN() != cfg.DeleteName {
			continue
		}
		target := c.GetPath(cfg.CollectionsPath)
		if target == "" {
			continue
		}
		if err := installer.RemoveCollectionDir(target); err != nil {
			utils.Error(err)
			return err
		}
		return nil
	}
	err := fmt.Errorf("%q is not in the requirements files", cfg.DeleteName)
	utils.Error(err)
	return err
}

// runUpdate drains the version-check channel to stdout and aggregates per-file errors into the returned error.
func runUpdate(cfg *config.Config, p *parser.Parser, files []parser.RequirementsFile) error {
	ch := make(chan parser.CheckProgress, 64)
	errCh := make(chan error, 1)
	go func() { errCh <- updateErrors(files, p.UpdateAll(files, ch)) }()
	var streamed bool
	for pr := range ch {
		switch {
		case pr.Notice != "":
			utils.Error(pr.Name, pr.Notice+", skipping")
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
func runInstall(cfg *config.Config, inst *installer.Installer, merged models.File, mergedColls models.Collections) error {
	ch := make(chan installer.Progress, 64)
	errCh := make(chan error, 1)
	go func() { errCh <- inst.InstallMissing(merged, mergedColls, ch) }()
	var streamed bool
	for pr := range ch {
		switch pr.Status {
		case "active":
			utils.Debug(cfg.Verbose, "installing", pr.Name, pr.Version)
		case "skipped":
			utils.Debug(cfg.Verbose, pr.Name, "(up to date)")
		case "unsupported":
			utils.Error(pr.Name, pr.Log+", skipping")
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
