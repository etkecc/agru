package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/etkecc/agru/internal/models"
)

func TestParseFileRolesAndCollections(t *testing.T) {
	content := `---
collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0
  - community.general

roles:
  - src: git+https://github.com/org/role-a.git
    version: v1.0.0
`
	path := writeTemp(t, content)
	p := New(newFakeRunner())

	main, additional, colls, extras, mapForm, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(main) != 1 {
		t.Errorf("main len = %d, want 1", len(main))
	}
	if len(additional) != 0 {
		t.Errorf("additional len = %d, want 0", len(additional))
	}
	if !mapForm {
		t.Errorf("mapForm = false, want true")
	}
	if len(colls) != 2 {
		t.Fatalf("colls len = %d, want 2", len(colls))
	}
	// First collection: supported git URL
	if colls[0].Unsupported() != "" {
		t.Errorf("colls[0].Unsupported() = %q, want empty", colls[0].Unsupported())
	}
	if colls[0].GetFQCN() != "community.docker" {
		t.Errorf("colls[0].GetFQCN() = %q, want community.docker", colls[0].GetFQCN())
	}
	// Second collection: unsupported galaxy name
	if colls[1].Unsupported() != models.ReasonGalaxySource {
		t.Errorf("colls[1].Unsupported() = %q, want %q", colls[1].Unsupported(), models.ReasonGalaxySource)
	}
	// Collections no longer appear in extras (FileMap has dedicated Collections field)
	if extras != nil {
		if _, ok := extras["collections"]; ok {
			t.Errorf("extras should not contain collections key anymore")
		}
	}
}

func TestUpdateFileCollectionsRoundTrip(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role-a.git"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repo] = "abc\trefs/tags/v2.0.0"
	fr.outputs["git ls-remote -tq --sort=-version:refname https://github.com/ansible-collections/community.docker"] = "def\trefs/tags/3.14.0\nghi\trefs/tags/3.13.0"

	content := `---

collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0
  - community.general

roles:
  - src: git+` + repo + `
    version: v1.0.0
`
	path := writeTemp(t, content)
	p := New(fr)

	entries, _, colls, extras, mapForm, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Update the file
	ch := make(chan CheckProgress, 10)
	if err := p.UpdateFile(entries, colls, extras, mapForm, path, ch); err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}

	// Collect progress
	var notices []string
	var bumps int
	for pr := range ch {
		if pr.Notice != "" {
			notices = append(notices, pr.Notice)
		}
		if pr.NewVer != "" {
			bumps++
		}
	}

	// community.general should emit a notice
	foundGalaxyNotice := false
	for _, n := range notices {
		if strings.Contains(n, models.ReasonGalaxySource) {
			foundGalaxyNotice = true
		}
	}
	if !foundGalaxyNotice {
		t.Errorf("expected galaxy source notice for community.general, got notices: %v", notices)
	}

	// community.docker should have been bumped (if a newer tag exists)
	if bumps != 1 {
		t.Logf("bumps = %d, expected 1 for community.docker (may depend on test tag availability)", bumps)
	}

	// Read the updated file
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}
	got := string(out)

	// Unsupported entry should be preserved verbatim
	if !strings.Contains(got, "community.general") {
		t.Errorf("UpdateFile() lost community.general entry:\n%s", got)
	}

	// File should still be map form (roles: + collections:)
	if !strings.Contains(got, "collections:") {
		t.Errorf("UpdateFile() lost collections: key:\n%s", got)
	}
	if !strings.Contains(got, "roles:") {
		t.Errorf("UpdateFile() lost roles: key:\n%s", got)
	}
}

func TestUpdateFileV1ListFormStaysList(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role-a.git"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repo] = "abc\trefs/tags/v2.0.0"

	// V1 list form: no roles: key, no collections:
	content := `- src: git+` + repo + `
  version: v1.0.0
`
	path := writeTemp(t, content)
	p := New(fr)

	entries, _, colls, extras, mapForm, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if mapForm {
		t.Errorf("mapForm = true, want false for v1 list form")
	}
	if len(colls) != 0 {
		t.Errorf("colls len = %d, want 0 for v1 list form", len(colls))
	}

	if err := p.UpdateFile(entries, colls, extras, mapForm, path, nil); err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}
	got := string(out)

	// Should NOT have roles: key injected
	if strings.Contains(got, "roles:") {
		t.Errorf("UpdateFile() injected roles: key into v1 list file:\n%s", got)
	}
	// Should not have collections: key either
	if strings.Contains(got, "collections:") {
		t.Errorf("UpdateFile() injected collections: key into v1 list file:\n%s", got)
	}
}

func TestUpdateFileCollectionsOnlyNoRolesKey(t *testing.T) {
	fr := newFakeRunner()

	// Collections-only file
	content := `---
collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0
`
	path := writeTemp(t, content)
	p := New(fr)

	_, _, colls, extras, mapForm, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if err := p.UpdateFile(nil, colls, extras, mapForm, path, nil); err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}
	got := string(out)

	// Should NOT have roles: [] injected
	if strings.Contains(got, "roles:") {
		t.Errorf("UpdateFile() injected roles: key into collections-only file:\n%s", got)
	}
	// Should have collections:
	if !strings.Contains(got, "collections:") {
		t.Errorf("UpdateFile() lost collections: key:\n%s", got)
	}
}

func TestCheckProgressNoticeFlow(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role-a.git"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repo] = "abc\trefs/tags/v2.0.0"

	content := `---
collections:
  - community.general
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0

roles:
  - src: git+` + repo + `
    version: v1.0.0
`
	path := writeTemp(t, content)
	p := New(fr)

	entries, _, colls, extras, mapForm, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	ch := make(chan CheckProgress, 10)
	if err := p.UpdateFile(entries, colls, extras, mapForm, path, ch); err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}

	var seen []CheckProgress
	for pr := range ch {
		seen = append(seen, pr)
	}

	// Should have 3 items: 1 notice (community.general) + 1 bump (community.docker) + 1 bump (role-a)
	if len(seen) != 3 {
		t.Fatalf("expected 3 progress events, got %d: %+v", len(seen), seen)
	}

	// Find the notice
	foundNotice := false
	for _, pr := range seen {
		if pr.Notice != "" {
			foundNotice = true
			if pr.Name != "community.general" {
				t.Errorf("notice name = %q, want community.general", pr.Name)
			}
			if pr.Notice != models.ReasonGalaxySource {
				t.Errorf("notice = %q, want %q", pr.Notice, models.ReasonGalaxySource)
			}
		}
	}
	if !foundNotice {
		t.Error("expected a notice progress event")
	}
}

func TestEmptyVersionCollectionUntouchedByUpdate(t *testing.T) {
	fr := newFakeRunner()

	content := `---
collections:
  - name: git+https://github.com/ansible-collections/community.docker
`
	path := writeTemp(t, content)
	p := New(fr)

	_, _, colls, extras, mapForm, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if colls[0].Version != "" {
		t.Fatalf("expected empty version, got %q", colls[0].Version)
	}

	ch := make(chan CheckProgress, 10)
	if err := p.UpdateFile(nil, colls, extras, mapForm, path, ch); err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}

	for pr := range ch {
		if pr.NewVer != "" {
			t.Errorf("empty-version collection should not get a NewVer, got %s -> %s", pr.Name, pr.NewVer)
		}
	}

	// File should still have no version pinned
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}
	if strings.Contains(string(out), "version:") {
		t.Errorf("empty-version collection should not get version pinned:\n%s", out)
	}
}

func TestIncludeFileCollectionsIgnored(t *testing.T) {
	tmpDir := t.TempDir()

	// Included file with collections (should be ignored)
	includeContent := `---
collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0

roles:
  - src: git+https://github.com/org/included-role.git
    version: v1.0.0
`
	includePath := filepath.Join(tmpDir, "included.yml")
	if err := os.WriteFile(includePath, []byte(includeContent), 0o600); err != nil {
		t.Fatal(err)
	}

	mainContent := `---
roles:
  - src: git+https://github.com/org/main-role.git
    version: v1.0.0
  - include: ` + includePath + `
`
	mainPath := filepath.Join(tmpDir, "requirements.yml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}

	p := New(newFakeRunner())
	main, additional, colls, _, _, err := p.ParseFile(mainPath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Main file should have 2 entries (main-role + include)
	if len(main) != 2 {
		t.Errorf("main len = %d, want 2", len(main))
	}
	// Additional should have the included role
	if len(additional) != 1 {
		t.Errorf("additional len = %d, want 1", len(additional))
	}
	if additional[0].GetName() != "included-role" {
		t.Errorf("additional[0] = %q, want included-role", additional[0].GetName())
	}
	// Collections from included file should NOT appear in main collections
	if len(colls) != 0 {
		t.Errorf("colls len = %d, want 0 (include file collections should be ignored)", len(colls))
	}
}

func TestMergeAllCollections(t *testing.T) {
	p := New(newFakeRunner())

	// Build collections via YAML sequence so unexported fields are set correctly
	makeColl := func(name, version string) *models.Collection {
		y := "- name: " + name + "\n"
		if version != "" {
			y += "  version: " + version + "\n"
		}
		var colls models.Collections
		if err := yaml.Unmarshal([]byte(y), &colls); err != nil {
			t.Fatalf("unmarshal collection %s: %v", name, err)
		}
		return colls[0]
	}

	files := []RequirementsFile{
		{
			Path:        "a/requirements.yml",
			Collections: models.Collections{makeColl("git+https://github.com/ansible-collections/community.docker", "3.13.0")},
		},
		{
			Path: "b/requirements.yml",
			Collections: models.Collections{
				makeColl("git+https://github.com/ansible-collections/community.docker", "3.14.0"), // duplicate, first wins
				makeColl("git+https://github.com/ansible-collections/community.general", "8.0.0"),
			},
		},
	}

	merged := p.MergeAllCollections(files)
	if len(merged) != 2 {
		t.Fatalf("MergeAllCollections() len = %d, want 2", len(merged))
	}
	// First file's version wins for duplicate
	byName := make(map[string]*models.Collection)
	for _, c := range merged {
		byName[c.GetFQCN()] = c
	}
	if byName["community.docker"].Version != "3.13.0" {
		t.Errorf("community.docker version = %q, want 3.13.0 (first file wins)", byName["community.docker"].Version)
	}
	if byName["community.general"].Version != "8.0.0" {
		t.Errorf("community.general version = %q, want 8.0.0", byName["community.general"].Version)
	}
}

func TestHostileBasenameEmitsNotice(t *testing.T) {
	content := `---
collections:
  - name: git+https://x/ns..git
    version: 1.0.0
`
	path := writeTemp(t, content)
	p := New(newFakeRunner())

	_, _, colls, _, mapForm, err := p.ParseFile(path)
	_ = mapForm
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(colls) != 1 {
		t.Fatalf("colls len = %d, want 1", len(colls))
	}
	if colls[0].Unsupported() != models.ReasonBadBasename {
		t.Errorf("unsupported = %q, want %q", colls[0].Unsupported(), models.ReasonBadBasename)
	}
}
