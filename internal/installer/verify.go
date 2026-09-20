package installer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
