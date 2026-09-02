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
	os.Args = []string{"agru", "-r", "a.yml", "-r", "b.yml", "-u"}
	cfg, _ := parseFlags()
	if cfg.UpdateFile != true {
		t.Errorf("UpdateFile should be true")
	}
	if cfg.RequirementsPath != "a.yml;b.yml" {
		t.Errorf("RequirementsPath = %q, want a.yml;b.yml", cfg.RequirementsPath)
	}
}

func TestParseFlagsShellExpansion(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	// Simulate shell expansion: -r file1 file2 -u
	os.Args = []string{"agru", "-r", "file1.yml", "file2.yml", "-u", "-no-tui"}
	cfg, _ := parseFlags()
	if !cfg.UpdateFile {
		t.Errorf("UpdateFile should be true")
	}
	if !cfg.NoTUI {
		t.Errorf("NoTUI should be true")
	}
	if cfg.RequirementsPath != "file1.yml;file2.yml" {
		t.Errorf("RequirementsPath = %q, want file1.yml;file2.yml", cfg.RequirementsPath)
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
