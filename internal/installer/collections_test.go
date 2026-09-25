package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
)

// newCollection builds a git-sourced collection entry the way requirements.yml parsing does.
func newCollection(t *testing.T, version string) *models.Collection {
	t.Helper()
	doc := "- name: git+https://github.com/ansible-collections/community.docker\n"
	if version != "" {
		doc += "  version: " + version + "\n"
	}
	var colls models.Collections
	if err := yaml.Unmarshal([]byte(doc), &colls); err != nil {
		t.Fatalf("parsing collection fixture: %v", err)
	}
	if len(colls) != 1 {
		t.Fatalf("got %d collections, want 1", len(colls))
	}
	return colls[0]
}

// dirMode returns the permission bits of p.
func dirMode(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat %s: %v", p, err)
	}
	return fi.Mode().Perm()
}

func TestExtractCollectionMarksTargetBeforeExtraction(t *testing.T) {
	collPath := t.TempDir()
	entry := newCollection(t, "3.13.0")
	galaxy := map[string]any{"namespace": "community", "name": "docker", "version": "3.13.0"}
	target := entry.GetPath(collPath)
	marker := filepath.Join(target, installMarkerName)

	interrupted := &Installer{runner: &callbackRunner{fn: func(command, _ string) (string, error) {
		if strings.HasPrefix(command, "tar -xf") {
			return "", errors.New("simulated interruption")
		}
		return "", nil
	}}}
	if err := interrupted.extractCollection(entry, galaxy, target, "archive.tar"); err == nil {
		t.Fatal("extractCollection() with a failing tar = nil, want an error")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("interrupted extraction left no install marker behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "MANIFEST.json")); !os.IsNotExist(err) {
		t.Errorf("interrupted extraction left a manifest, which reads as installed: stat err = %v", err)
	}
	if err := RemoveCollectionDir(target); err != nil {
		t.Fatalf("interrupted install left a dir that cannot be replaced: %v", err)
	}

	ok := &Installer{runner: &callbackRunner{fn: func(command, _ string) (string, error) {
		if strings.HasPrefix(command, "tar -xf") {
			if err := os.WriteFile(filepath.Join(target, "galaxy.yml"), []byte("namespace: community\n"), 0o600); err != nil {
				return "", err
			}
		}
		return "", nil
	}}}
	if err := ok.extractCollection(entry, galaxy, target, "archive.tar"); err != nil {
		t.Fatalf("extractCollection() error = %v", err)
	}
	info, err := models.ParseManifest(mustRead(t, filepath.Join(target, "MANIFEST.json")))
	if err != nil {
		t.Fatalf("extracted manifest is unreadable: %v", err)
	}
	if info["version"] != "3.13.0" {
		t.Errorf("manifest version = %v, want 3.13.0", info["version"])
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("successful extraction left the install marker behind: stat err = %v", err)
	}
	if got := dirMode(t, target); got != 0o700 {
		t.Errorf("collection dir mode = %o, want 700", got)
	}
	if got := dirMode(t, filepath.Dir(target)); got != 0o755 {
		t.Errorf("namespace dir mode = %o, want 755", got)
	}
}

// mustRead returns the contents of p or fails the test.
func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading %s: %v", p, err)
	}
	return data
}
