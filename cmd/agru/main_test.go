package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFlagsDefaults(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"agru"}
	cfg, show := parseFlags()
	if show {
		t.Fatalf("showVersion should be false")
	}
	if cfg.RequirementsPath != "requirements.yml" {
		t.Errorf("default RequirementsPath = %q, want requirements.yml", cfg.RequirementsPath)
	}
	if cfg.RolesPath != "roles/galaxy/" {
		t.Errorf("default RolesPath = %q", cfg.RolesPath)
	}
	if cfg.CollectionsPath == "" {
		t.Errorf("default CollectionsPath should not be empty")
	}
}

func TestParseFlagsRepeatedR(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	tmp := t.TempDir()
	a := tmp + "/a.yml"
	b := tmp + "/b.yml"
	if err := os.WriteFile(a, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"agru", "-r", a, "-r", b, "-u"}
	cfg, _ := parseFlags()
	if cfg.UpdateFile != true {
		t.Errorf("UpdateFile should be true")
	}
	if cfg.RequirementsPath != a+";"+b {
		t.Errorf("RequirementsPath = %q, want %q", cfg.RequirementsPath, a+";"+b)
	}
}

func TestParseFlagsShellExpansion(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	tmp := t.TempDir()
	f1 := tmp + "/file1.yml"
	f2 := tmp + "/file2.yml"
	if err := os.WriteFile(f1, []byte("f1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("f2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate shell expansion: -r file1 file2 -u
	os.Args = []string{"agru", "-r", f1, f2, "-u", "-no-tui"}
	cfg, _ := parseFlags()
	if !cfg.UpdateFile {
		t.Errorf("UpdateFile should be true")
	}
	if !cfg.NoTUI {
		t.Errorf("NoTUI should be true")
	}
	if cfg.RequirementsPath != f1+";"+f2 {
		t.Errorf("RequirementsPath = %q, want %q", cfg.RequirementsPath, f1+";"+f2)
	}
}

func TestParseFlagsIgnoresNonExistentFiles(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	tmp := t.TempDir()
	exists := tmp + "/exists.yml"
	if err := os.WriteFile(exists, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"agru", "-r", exists, "-r", "does-not-exist.yml"}
	cfg, _ := parseFlags()
	if cfg.RequirementsPath != exists {
		t.Errorf("RequirementsPath = %q, want only existing file %q", cfg.RequirementsPath, exists)
	}
}

func TestParseFlagsVersion(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"agru", "-v"}
	_, show := parseFlags()
	if !show {
		t.Errorf("showVersion should be true")
	}
}

func TestParseFlagsCollectionsPath(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	// -cp flag sets collections path
	os.Args = []string{"agru", "-cp", "/custom/collections"}
	cfg, _ := parseFlags()
	if cfg.CollectionsPath != "/custom/collections" {
		t.Errorf("CollectionsPath = %q, want /custom/collections", cfg.CollectionsPath)
	}
}

func TestResolveCollectionsPathEnvPrecedence(t *testing.T) {
	// Env overrides default
	t.Setenv("ANSIBLE_COLLECTIONS_PATH", "/env/collections")
	path, err := resolveCollectionsPath("")
	if err != nil {
		t.Fatalf("resolveCollectionsPath error: %v", err)
	}
	if path != "/env/collections" {
		t.Errorf("resolveCollectionsPath = %q, want /env/collections", path)
	}
}

func TestResolveCollectionsPathLegacyEnv(t *testing.T) {
	t.Setenv("ANSIBLE_COLLECTIONS_PATHS", "/legacy/collections:/other")
	path, err := resolveCollectionsPath("")
	if err != nil {
		t.Fatalf("resolveCollectionsPath error: %v", err)
	}
	// Should use first entry
	if path != "/legacy/collections" {
		t.Errorf("resolveCollectionsPath = %q, want /legacy/collections", path)
	}
}

func TestResolveCollectionsPathDefault(t *testing.T) {
	path, err := resolveCollectionsPath("")
	if err != nil {
		t.Fatalf("resolveCollectionsPath error: %v", err)
	}
	if !strings.HasSuffix(path, ".ansible/collections") {
		t.Errorf("resolveCollectionsPath = %q, want suffix .ansible/collections", path)
	}
}

func TestResolveCollectionsPathFlagTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path, err := resolveCollectionsPath("~/my/collections")
	if err != nil {
		t.Fatalf("resolveCollectionsPath error: %v", err)
	}
	expected := filepath.Join(home, "my", "collections")
	if path != expected {
		t.Errorf("resolveCollectionsPath = %q, want %q", path, expected)
	}
}
