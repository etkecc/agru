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

// collectionDir builds a directory at the canonical ansible_collections layout the guard expects.
func collectionDir(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name, "ansible_collections", "community", "docker")
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o755); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return dir
}

// writeMarker drops install-marker content into dir, optionally behind a symlink.
func writeMarker(t *testing.T, dir, content string, symlink bool) {
	t.Helper()
	src := filepath.Join(dir, installMarkerName)
	if !symlink {
		if err := os.WriteFile(src, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(src+"-target", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src+"-target", src); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveCollectionDirGuards(t *testing.T) {
	tmp := t.TempDir()
	matching := `{"collection_info":{"namespace":"community","name":"docker","version":"3.13.0"},"format":"1.0.0"}`

	installed := collectionDir(t, tmp, "installed")
	if err := os.WriteFile(filepath.Join(installed, "MANIFEST.json"), []byte(matching), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCollectionDir(installed); err != nil {
		t.Fatalf("RemoveCollectionDir(installed) error = %v", err)
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Errorf("collection dir still on disk: stat err = %v", err)
	}

	for _, tt := range []struct {
		name    string
		write   bool
		content string
	}{
		{"no manifest", false, ""},
		{"empty manifest", true, ""},
		{"manifest not json", true, "not json"},
		{"manifest without collection_info", true, `{"format":"1.0.0"}`},
		{"manifest with empty collection_info", true, `{"collection_info":{}}`},
		{"manifest naming another collection", true, `{"collection_info":{"namespace":"other","name":"thing"}}`},
		{"manifest without namespace", true, `{"collection_info":{"name":"docker"}}`},
	} {
		dir := collectionDir(t, tmp, tt.name)
		if tt.write {
			if err := os.WriteFile(filepath.Join(dir, "MANIFEST.json"), []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := RemoveCollectionDir(dir); err == nil {
			t.Errorf("RemoveCollectionDir(%s) = nil, want a refusal", tt.name)
		}
		if _, err := os.Stat(filepath.Join(dir, "plugins")); err != nil {
			t.Errorf("%s: refused but the dir is gone: %v", tt.name, err)
		}
	}

	// a symlinked manifest must not count as an installed collection
	symlinked := collectionDir(t, tmp, "symlinked")
	outside := filepath.Join(tmp, "outside.json")
	if err := os.WriteFile(outside, []byte(matching), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(symlinked, "MANIFEST.json")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCollectionDir(symlinked); err == nil {
		t.Error("RemoveCollectionDir(symlinked manifest) = nil, want a refusal")
	}

	// agru's own in-progress marker authorizes replacing a collection it was installing
	marked := collectionDir(t, tmp, "marked")
	if err := os.WriteFile(filepath.Join(marked, installMarkerName), []byte("community.docker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCollectionDir(marked); err != nil {
		t.Errorf("RemoveCollectionDir(in-progress collection) error = %v, want nil", err)
	}

	for _, tt := range []struct {
		name    string
		write   bool
		content string
		symlink bool
	}{
		{"marker naming another collection", true, "other.thing", false},
		{"empty marker", true, "", false},
		{"symlinked marker", true, "community.docker", true},
		{"no marker", false, "", false},
	} {
		dir := collectionDir(t, tmp, tt.name)
		if tt.write {
			writeMarker(t, dir, tt.content, tt.symlink)
		}
		if err := RemoveCollectionDir(dir); err == nil {
			t.Errorf("RemoveCollectionDir(%s) = nil, want a refusal", tt.name)
		}
		if _, err := os.Stat(filepath.Join(dir, "plugins")); err != nil {
			t.Errorf("%s: refused but the dir is gone: %v", tt.name, err)
		}
	}

	if err := RemoveCollectionDir(filepath.Join(tmp, "absent")); err != nil {
		t.Errorf("RemoveCollectionDir(missing dir) = %v, want nil", err)
	}
}

func TestRemoveRoleDirGuards(t *testing.T) {
	tmp := t.TempDir()
	role := filepath.Join(tmp, "role-a")
	if err := os.MkdirAll(filepath.Join(role, "meta"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(role, "meta", "main.yml"), []byte("galaxy_info:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoleDir(role); err != nil {
		t.Fatalf("RemoveRoleDir(role) error = %v", err)
	}
	if _, err := os.Stat(role); !os.IsNotExist(err) {
		t.Errorf("role dir still on disk after removal: stat err = %v", err)
	}

	decoy := filepath.Join(tmp, "cache")
	if err := os.MkdirAll(filepath.Join(decoy, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoleDir(decoy); err == nil {
		t.Error("RemoveRoleDir(non-role dir) = nil, want a refusal")
	}
	if _, err := os.Stat(filepath.Join(decoy, "sub")); err != nil {
		t.Errorf("refusal still touched the decoy: stat err = %v", err)
	}

	// a symlinked marker or a symlinked meta dir must not pass as role metadata
	outside := filepath.Join(tmp, "outside.yml")
	if err := os.WriteFile(outside, []byte("galaxy_info:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkedMarker := filepath.Join(tmp, "linked-marker")
	if err := os.MkdirAll(filepath.Join(linkedMarker, "meta"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(linkedMarker, "meta", "main.yml")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoleDir(linkedMarker); err == nil {
		t.Error("RemoveRoleDir(symlinked marker) = nil, want a refusal")
	}
	linkedMeta := filepath.Join(tmp, "linked-meta")
	if err := os.MkdirAll(linkedMeta, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(linkedMarker, "meta"), filepath.Join(linkedMeta, "meta")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoleDir(linkedMeta); err == nil {
		t.Error("RemoveRoleDir(symlinked meta dir) = nil, want a refusal")
	}

	// an empty marker is not role metadata either
	emptyMarker := filepath.Join(tmp, "empty-marker")
	if err := os.MkdirAll(filepath.Join(emptyMarker, "meta"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyMarker, "meta", "main.yml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRoleDir(emptyMarker); err == nil {
		t.Error("RemoveRoleDir(empty marker) = nil, want a refusal")
	}

	if err := RemoveRoleDir(filepath.Join(tmp, "absent")); err != nil {
		t.Errorf("RemoveRoleDir(missing dir) = %v, want nil", err)
	}
}
