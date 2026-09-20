package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyRoleTree(t *testing.T) {
	writeFile := func(t *testing.T, path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("clean tree", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "meta"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, "meta", "main.yml"))
		if err := verifyRoleTree(root); err != nil {
			t.Errorf("verifyRoleTree() = %v, want nil", err)
		}
	})

	t.Run("internal relative symlink", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "a"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("a", filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		if err := verifyRoleTree(root); err != nil {
			t.Errorf("verifyRoleTree() = %v, want nil", err)
		}
	})

	t.Run("internal absolute symlink", func(t *testing.T) {
		root := t.TempDir()
		inner := filepath.Join(root, "inner")
		if err := os.MkdirAll(inner, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(inner, filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		if err := verifyRoleTree(root); err != nil {
			t.Errorf("verifyRoleTree() = %v, want nil", err)
		}
	})

	t.Run("escaping relative symlink", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink("../../outside", filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		err := verifyRoleTree(root)
		if err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Errorf("verifyRoleTree() = %v, want escapes error", err)
		}
	})

	t.Run("escaping chained symlink", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink("b", filepath.Join(root, "a")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../outside", filepath.Join(root, "b")); err != nil {
			t.Fatal(err)
		}
		err := verifyRoleTree(root)
		if err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Errorf("verifyRoleTree() = %v, want escapes error", err)
		}
	})

	t.Run("escaping depth-shifting tower", func(t *testing.T) {
		// Tower: links stay one level deep physically but look L+1 deep lexically, so L+1 dots escape root.
		root := t.TempDir()
		outside := filepath.Join(filepath.Dir(root), "outside.txt")
		if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		const L = 10
		if err := os.MkdirAll(filepath.Join(root, "meta"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../c1", filepath.Join(root, "meta", "b")); err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= L; i++ {
			dir := filepath.Join(root, fmt.Sprintf("c%d", i))
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if i < L {
				if err := os.Symlink(fmt.Sprintf("../c%d", i+1), filepath.Join(dir, "x")); err != nil {
					t.Fatal(err)
				}
			}
		}
		target := "b" + strings.Repeat("/x", L-1) + strings.Repeat("/..", L+1) + "/outside.txt"
		if err := os.Symlink(target, filepath.Join(root, "meta", ".galaxy_install_info")); err != nil {
			t.Fatal(err)
		}
		err := verifyRoleTree(root)
		if err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Errorf("verifyRoleTree() = %v, want escapes error for the tower", err)
		}
	})

	t.Run("absolute escaping symlink", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink("/etc", filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		err := verifyRoleTree(root)
		if err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Errorf("verifyRoleTree() = %v, want escapes error", err)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		if err := verifyRoleTree(filepath.Join(t.TempDir(), "does-not-exist")); err != nil {
			t.Errorf("verifyRoleTree() = %v, want nil", err)
		}
	})
}

func TestAssertRegularDir(t *testing.T) {
	t.Run("real dir", func(t *testing.T) {
		if err := assertRegularDir(t.TempDir()); err != nil {
			t.Errorf("assertRegularDir() = %v, want nil", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		if err := assertRegularDir(filepath.Join(root, "link")); err == nil {
			t.Error("assertRegularDir() = nil, want error for symlink")
		}
	})

	t.Run("file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := assertRegularDir(f); err == nil {
			t.Error("assertRegularDir() = nil, want error for file")
		}
	})

	t.Run("missing", func(t *testing.T) {
		if err := assertRegularDir(filepath.Join(t.TempDir(), "nope")); err == nil {
			t.Error("assertRegularDir() = nil, want error for missing path")
		}
	})
}
