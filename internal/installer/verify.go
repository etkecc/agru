package installer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/etkecc/agru/internal/models"
)

// verifyRoleTree rejects a tree containing symlinks that escape root.
func verifyRoleTree(root string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", root, err)
	}
	// A missing root is treated as empty: fake runners in tests clone to a dir that is never created.
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", root, err)
	}
	// Physical root for the EvalSymlinks pass: Abs leaves symlinked prefixes (e.g. /tmp) unresolved.
	rootPhys, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", root, err)
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(p)
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", p, err)
		}
		return checkSymlink(p, target, rootAbs, rootPhys)
	})
}

// checkSymlink rejects a symlink whose lexical or physical resolution leaves root.
func checkSymlink(p, target, rootAbs, rootPhys string) error {
	// Lexical pass: a single hop whose cleaned target leaves root.
	resolved := target
	if !filepath.IsAbs(target) {
		resolved = filepath.Join(filepath.Dir(p), target)
	}
	rel, err := filepath.Rel(rootAbs, filepath.Clean(resolved))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("symlink %s escapes tree (target: %s)", p, target)
	}
	// Physical pass: chains resolve like the kernel does; dangling/looping chains are escapes.
	phys, err := filepath.EvalSymlinks(p)
	if err != nil {
		return fmt.Errorf("symlink %s escapes tree (unresolvable chain, target: %s)", p, target)
	}
	if rel, err = filepath.Rel(rootPhys, phys); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("symlink %s escapes tree (resolves to %s)", p, phys)
	}
	return nil
}

// assertRegularDir rejects p when it is a symlink or not a directory.
func assertRegularDir(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", p)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", p)
	}
	return nil
}

// RemoveCollectionDir removes an installed collection dir, refusing to touch anything that is not one.
func RemoveCollectionDir(dir string) error {
	if _, err := os.Lstat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !isCollectionDir(dir) {
		return fmt.Errorf("refusing to remove %s: not an installed collection (no MANIFEST.json naming it), remove it manually first", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing collection dir %s: %w", dir, err)
	}
	return nil
}

// isCollectionDir reports whether dir holds that collection, installed or marked as agru's own.
func isCollectionDir(dir string) bool {
	dir = filepath.Clean(dir)
	ns := filepath.Base(filepath.Dir(dir))
	coll := filepath.Base(dir)
	if data, ok := readRegularFile(filepath.Join(dir, "MANIFEST.json")); ok {
		if info, err := models.ParseManifest(data); err == nil {
			manifestNS, nsok := info["namespace"].(string)
			manifestColl, collok := info["name"].(string)
			if nsok && collok && manifestNS == ns && manifestColl == coll {
				return true
			}
		}
	}
	data, ok := readRegularFile(filepath.Join(dir, installMarkerName))
	return ok && strings.TrimSpace(string(data)) == ns+"."+coll
}

// readRegularFile reads p only when it is a non-symlinked, non-empty regular file.
func readRegularFile(p string) ([]byte, bool) {
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
		return nil, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	return data, true
}

// RemoveRoleDir removes an installed role dir, refusing to touch anything that is not one.
func RemoveRoleDir(dir string) error {
	if _, err := os.Lstat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !isRoleDir(dir) {
		return fmt.Errorf("refusing to remove %s: not a role (no meta/main.yml or meta/.galaxy_install_info), remove it manually first", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing role dir %s: %w", dir, err)
	}
	return nil
}

// isRoleDir reports whether dir carries role metadata written by ansible or agru.
func isRoleDir(dir string) bool {
	if assertRegularDir(filepath.Join(dir, "meta")) != nil {
		return false
	}
	for _, marker := range []string{"main.yml", "main.yaml", ".galaxy_install_info"} {
		fi, err := os.Lstat(filepath.Join(dir, "meta", marker))
		if err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
			return true
		}
	}
	return false
}
