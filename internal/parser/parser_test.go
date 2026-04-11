package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/etkecc/agru/internal/models"
)

// fakeRunner records calls and returns preset outputs
type fakeRunner struct {
	outputs map[string]string
	errors  map[string]error
	calls   []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		outputs: make(map[string]string),
		errors:  make(map[string]error),
	}
}

func (r *fakeRunner) Run(command, _ string) (string, error) {
	r.calls = append(r.calls, command)
	if err, ok := r.errors[command]; ok {
		return r.outputs[command], err
	}
	return r.outputs[command], nil
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "requirements-*.yml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestParseFileDirectList(t *testing.T) {
	content := `- src: git+https://github.com/org/role-a.git
  version: v1.0.0
- src: git+https://github.com/org/role-b.git
  version: v2.0.0
  name: custom-name
`
	path := writeTemp(t, content)
	p := New(newFakeRunner())

	main, additional, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(additional) != 0 {
		t.Errorf("ParseFile() additional len = %d, want 0", len(additional))
	}
	if len(main) != 2 {
		t.Fatalf("ParseFile() main len = %d, want 2", len(main))
	}
	// Sorted alphabetically: custom-name before role-a
	if main[0].GetName() != "custom-name" {
		t.Errorf("ParseFile() main[0].GetName() = %q, want custom-name", main[0].GetName())
	}
	if main[1].GetName() != "role-a" {
		t.Errorf("ParseFile() main[1].GetName() = %q, want role-a", main[1].GetName())
	}
}

func TestParseFileMapFormat(t *testing.T) {
	content := `roles:
  - src: git+https://github.com/org/role-a.git
    version: v1.0.0
  - src: git+https://github.com/org/role-b.git
    version: v2.0.0
`
	path := writeTemp(t, content)
	p := New(newFakeRunner())

	main, _, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(main) != 2 {
		t.Errorf("ParseFile() main len = %d, want 2", len(main))
	}
}

func TestParseFileDeduplicate(t *testing.T) {
	content := `- src: git+https://github.com/org/role-a.git
  version: v1.0.0
- src: git+https://github.com/org/role-a.git
  version: v2.0.0
`
	path := writeTemp(t, content)
	p := New(newFakeRunner())

	main, _, err := p.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(main) != 1 {
		t.Errorf("ParseFile() main len = %d, want 1 (deduplicated)", len(main))
	}
}

func TestParseFileWithInclude(t *testing.T) {
	tmpDir := t.TempDir()

	additionalContent := `- src: git+https://github.com/org/included-role.git
  version: v3.0.0
`
	additionalPath := filepath.Join(tmpDir, "additional.yml")
	if err := os.WriteFile(additionalPath, []byte(additionalContent), 0o600); err != nil {
		t.Fatal(err)
	}

	mainContent := "- src: git+https://github.com/org/main-role.git\n  version: v1.0.0\n- include: " + additionalPath + "\n"
	mainPath := filepath.Join(tmpDir, "requirements.yml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}

	p := New(newFakeRunner())
	main, additional, err := p.ParseFile(mainPath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if len(main) != 2 {
		t.Errorf("ParseFile() main len = %d, want 2 (role + include entry)", len(main))
	}
	if len(additional) != 1 {
		t.Errorf("ParseFile() additional len = %d, want 1", len(additional))
	}
	if additional[0].GetName() != "included-role" {
		t.Errorf("ParseFile() additional[0].GetName() = %q, want included-role", additional[0].GetName())
	}
}

func TestParseFileNotFound(t *testing.T) {
	p := New(newFakeRunner())
	_, _, err := p.ParseFile("/nonexistent/requirements.yml")
	if err == nil {
		t.Error("ParseFile() expected error for missing file, got nil")
	}
}

func TestGetNewVersionSkipsIgnored(t *testing.T) {
	p := New(newFakeRunner())

	for _, version := range []string{"main", "master"} {
		newVer, err := p.getNewVersion("git+https://github.com/org/role.git", version)
		if err != nil {
			t.Errorf("getNewVersion(%q) error = %v", version, err)
		}
		if newVer != "" {
			t.Errorf("getNewVersion(%q) = %q, want empty (ignored version)", version, newVer)
		}
	}
}

func TestGetNewVersionSkipsNonGit(t *testing.T) {
	p := New(newFakeRunner())
	newVer, err := p.getNewVersion("https://example.com/role.tar.gz", "v1.0.0")
	if err != nil {
		t.Errorf("getNewVersion() error = %v", err)
	}
	if newVer != "" {
		t.Errorf("getNewVersion() = %q, want empty for non-git src", newVer)
	}
}

func TestGetNewVersionReturnsNewTag(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role.git"
	cmd := "git ls-remote -tq --sort=-version:refname " + repo
	fr.outputs[cmd] = "abc123\trefs/tags/v2.0.0\ndef456\trefs/tags/v1.0.0"

	p := New(fr)
	newVer, err := p.getNewVersion("git+"+repo, "v1.0.0")
	if err != nil {
		t.Fatalf("getNewVersion() error = %v", err)
	}
	if newVer != "v2.0.0" {
		t.Errorf("getNewVersion() = %q, want v2.0.0", newVer)
	}
}

func TestGetNewVersionSameVersion(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role.git"
	cmd := "git ls-remote -tq --sort=-version:refname " + repo
	fr.outputs[cmd] = "abc123\trefs/tags/v1.0.0"

	p := New(fr)
	newVer, err := p.getNewVersion("git+"+repo, "v1.0.0")
	if err != nil {
		t.Fatalf("getNewVersion() error = %v", err)
	}
	if newVer != "" {
		t.Errorf("getNewVersion() = %q, want empty when already at latest", newVer)
	}
}

func TestGetNewVersionHandlesCurlyBrace(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role.git"
	cmd := "git ls-remote -tq --sort=-version:refname " + repo
	// Some GitHub repos append ^{} to tag refs
	fr.outputs[cmd] = "abc123\trefs/tags/v2.0.0^{}\ndef456\trefs/tags/v1.0.0"

	p := New(fr)
	newVer, err := p.getNewVersion("git+"+repo, "v1.0.0")
	if err != nil {
		t.Fatalf("getNewVersion() error = %v", err)
	}
	if newVer != "v2.0.0" {
		t.Errorf("getNewVersion() = %q, want v2.0.0 (^{} stripped)", newVer)
	}
}

func TestUpdateFile(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role-a.git"
	cmd := "git ls-remote -tq --sort=-version:refname " + repo
	fr.outputs[cmd] = "abc\trefs/tags/v2.0.0\ndef\trefs/tags/v1.0.0"

	entries := models.File{
		{Src: "git+" + repo, Version: "v1.0.0"},
	}
	// Manually set name to avoid path.Base parsing
	entries[0].Name = "role-a"

	tmpPath := writeTemp(t, "")
	p := New(fr)

	if err := p.UpdateFile(entries, tmpPath, nil); err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}

	// Entry version should be updated in place
	if entries[0].Version != "v2.0.0" {
		t.Errorf("UpdateFile() entry version = %q, want v2.0.0", entries[0].Version)
	}

	// File should be written
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("reading updated file: %v", err)
	}
	if !strings.HasPrefix(string(content), "---\n") {
		t.Errorf("UpdateFile() file content should start with ---, got: %q", string(content[:20]))
	}
}

func TestMergeFiles(t *testing.T) {
	main := models.File{
		{Name: "role-a", Version: "v1.0.0"},
		{Name: "role-b", Version: "v2.0.0"},
	}
	additional := models.File{
		{Name: "role-b", Version: "v99.0.0"}, // should be overridden by main
		{Name: "role-c", Version: "v3.0.0"},
	}

	p := New(newFakeRunner())
	result := p.MergeFiles(main, additional)

	if len(result) != 3 {
		t.Fatalf("MergeFiles() len = %d, want 3", len(result))
	}

	byName := make(map[string]*models.Entry)
	for _, e := range result {
		byName[e.GetName()] = e
	}

	if byName["role-b"].Version != "v2.0.0" {
		t.Errorf("MergeFiles() role-b version = %q, want v2.0.0 (main wins)", byName["role-b"].Version)
	}
	if byName["role-c"].Version != "v3.0.0" {
		t.Errorf("MergeFiles() role-c version = %q, want v3.0.0", byName["role-c"].Version)
	}
}

func TestMergeFilesSorted(t *testing.T) {
	main := models.File{
		{Name: "zebra"},
		{Name: "alpha"},
	}
	additional := models.File{
		{Name: "mango"},
	}

	p := New(newFakeRunner())
	result := p.MergeFiles(main, additional)

	if result[0].GetName() != "alpha" || result[1].GetName() != "mango" || result[2].GetName() != "zebra" {
		t.Errorf("MergeFiles() not sorted: got %v", func() []string {
			names := make([]string, len(result))
			for i, e := range result {
				names[i] = e.GetName()
			}
			return names
		}())
	}
}
