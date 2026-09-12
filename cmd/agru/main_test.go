package main

import (
	"os"
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
