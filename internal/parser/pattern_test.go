package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/etkecc/agru/internal/models"
)

// writeTree builds dir/molecule/{mariadb,postgres,redis}/requirements.yml plus a two-level deep/a file and a decoy.
func writeTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mk := func(rel, role string) {
		fp := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		content := "- src: git+https://github.com/org/" + role + ".git\n  version: v1.0.0\n"
		if err := os.WriteFile(fp, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", fp, err)
		}
	}
	mk("molecule/mariadb/requirements.yml", "role-mariadb")
	mk("molecule/postgres/requirements.yml", "role-postgres")
	mk("molecule/redis/requirements.yml", "role-redis")
	mk("molecule/deep/a/requirements.yml", "role-deep")
	if err := os.WriteFile(filepath.Join(dir, "molecule", "notes.txt"), []byte("not a requirements file"), 0o600); err != nil {
		t.Fatalf("writing decoy: %v", err)
	}
	return dir
}

func TestParsePatternDoubleStar(t *testing.T) {
	dir := writeTree(t)
	p := New(newFakeRunner())

	files, err := p.ParsePattern(filepath.Join(dir, "molecule", "**", "requirements.yml"))
	if err != nil {
		t.Fatalf("ParsePattern() error = %v", err)
	}
	// merge precedence follows this sort order, so the sequence is asserted below
	want := []string{
		filepath.Join("molecule", "deep", "a", "requirements.yml"),
		filepath.Join("molecule", "mariadb", "requirements.yml"),
		filepath.Join("molecule", "postgres", "requirements.yml"),
		filepath.Join("molecule", "redis", "requirements.yml"),
	}
	if len(files) != len(want) {
		t.Fatalf("ParsePattern() matched %d files, want %d", len(files), len(want))
	}
	for i, f := range files {
		if !strings.HasSuffix(f.Path, want[i]) {
			t.Errorf("ParsePattern() files[%d] = %q, want suffix %q", i, f.Path, want[i])
		}
		if len(f.Entries) != 1 {
			t.Errorf("ParsePattern() files[%d] has %d entries, want 1", i, len(f.Entries))
		}
	}
}

func TestParsePatternSingleStarStaysOneLevel(t *testing.T) {
	dir := writeTree(t)
	p := New(newFakeRunner())

	files, err := p.ParsePattern(filepath.Join(dir, "molecule", "*", "requirements.yml"))
	if err != nil {
		t.Fatalf("ParsePattern() error = %v", err)
	}
	// deep/a/requirements.yml is two levels down; a single * must not reach it
	if len(files) != 3 {
		t.Fatalf("ParsePattern() matched %d files, want 3", len(files))
	}
}

func TestParsePatternLiteralMissing(t *testing.T) {
	dir := t.TempDir()
	p := New(newFakeRunner())

	_, err := p.ParsePattern(filepath.Join(dir, "requirements.yml"))
	if err == nil || !strings.Contains(err.Error(), "reading file") {
		t.Fatalf("ParsePattern() on missing literal = %v, want the original reading file error", err)
	}
}

func TestParsePatternNoMatch(t *testing.T) {
	dir := t.TempDir()
	p := New(newFakeRunner())

	_, err := p.ParsePattern(filepath.Join(dir, "nothing", "**", "*.yml"))
	if err == nil || !strings.Contains(err.Error(), "no requirements files match pattern") {
		t.Fatalf("ParsePattern() with no matches = %v, want no-match error", err)
	}
}

func TestParsePatternSkipsDirectories(t *testing.T) {
	dir := writeTree(t)
	if err := os.MkdirAll(filepath.Join(dir, "molecule", "mariadb", "requirements.yml.bak"), 0o755); err != nil {
		t.Fatalf("mkdir decoy dir: %v", err)
	}
	p := New(newFakeRunner())

	files, err := p.ParsePattern(filepath.Join(dir, "molecule", "**", "requirements.yml*"))
	if err != nil {
		t.Fatalf("ParsePattern() error = %v", err)
	}
	for _, f := range files {
		info, statErr := os.Stat(f.Path)
		if statErr != nil || !info.Mode().IsRegular() {
			t.Errorf("ParsePattern() returned non-regular file %q", f.Path)
		}
	}
}

func TestParsePatternMultiplePatterns(t *testing.T) {
	dir := writeTree(t)
	p := New(newFakeRunner())

	pattern := filepath.Join(dir, "molecule", "mariadb", "requirements.yml") + ";" + filepath.Join(dir, "molecule", "redis", "requirements.yml")
	files, err := p.ParsePattern(pattern)
	if err != nil {
		t.Fatalf("ParsePattern() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ParsePattern() matched %d files, want 2", len(files))
	}
	paths := []string{files[0].Path, files[1].Path}
	if !strings.HasSuffix(paths[0], "mariadb/requirements.yml") || !strings.HasSuffix(paths[1], "redis/requirements.yml") {
		t.Errorf("ParsePattern() paths = %v, want mariadb and redis", paths)
	}
}

func TestMergeAll(t *testing.T) {
	p := New(newFakeRunner())
	files := []RequirementsFile{
		{
			Path:    "a/requirements.yml",
			Entries: models.File{{Name: "role-a", Version: "v1.0.0"}, {Name: "role-b", Version: "v1.0.0"}},
		},
		{
			Path:       "b/requirements.yml",
			Entries:    models.File{{Name: "role-b", Version: "v99.0.0"}, {Name: "role-c", Version: "v2.0.0"}},
			Additional: models.File{{Name: "role-d", Version: "v3.0.0"}},
		},
	}

	merged := p.MergeAll(files)
	if len(merged) != 4 {
		t.Fatalf("MergeAll() len = %d, want 4", len(merged))
	}
	byName := make(map[string]*models.Entry)
	for _, e := range merged {
		byName[e.GetName()] = e
	}
	if byName["role-b"].Version != "v1.0.0" {
		t.Errorf("MergeAll() role-b = %q, want v1.0.0 (earlier file wins)", byName["role-b"].Version)
	}
	if byName["role-c"].Version != "v2.0.0" {
		t.Errorf("MergeAll() role-c = %q, want v2.0.0", byName["role-c"].Version)
	}
	if byName["role-d"].Version != "v3.0.0" {
		t.Errorf("MergeAll() role-d = %q, want v3.0.0 (include entries included)", byName["role-d"].Version)
	}
}

func TestUpdateAll(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role-a.git"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repo] = "abc\trefs/tags/v2.0.0"

	f1 := writeTemp(t, "- src: git+"+repo+"\n  version: v1.0.0\n")
	f2 := writeTemp(t, "- src: git+"+repo+"\n  version: v1.0.0\n")
	p := New(fr)
	entries := func() models.File { return models.File{{Name: "role-a", Src: "git+" + repo, Version: "v1.0.0"}} }

	ch := make(chan CheckProgress, 64)
	errs := p.UpdateAll([]RequirementsFile{{Path: f1, Entries: entries()}, {Path: f2, Entries: entries()}}, ch)
	var events int
	for range ch {
		events++
	}
	if len(errs) != 2 || errs[0] != nil || errs[1] != nil {
		t.Fatalf("UpdateAll() errs = %v, want all nil", errs)
	}
	if events != 2 {
		t.Errorf("UpdateAll() progress events = %d, want 2 (one per role, one per file)", events)
	}
	for i, f := range []string{f1, f2} {
		out, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		if !strings.Contains(string(out), "v2.0.0") {
			t.Errorf("UpdateAll() file %d not rewritten:\n%s", i+1, out)
		}
	}
}

func TestUpdateAllWriteError(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role-a.git"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repo] = "abc\trefs/tags/v2.0.0"

	good := writeTemp(t, "- src: git+"+repo+"\n  version: v1.0.0\n")
	bad := t.TempDir() // a directory: os.WriteFile must fail on it, root or not
	p := New(fr)
	entries := func() models.File { return models.File{{Name: "role-a", Src: "git+" + repo, Version: "v1.0.0"}} }

	ch := make(chan CheckProgress, 64)
	errs := p.UpdateAll([]RequirementsFile{{Path: good, Entries: entries()}, {Path: bad, Entries: entries()}}, ch)
	var events int
	for range ch {
		events++
	}
	if events != 2 {
		t.Errorf("UpdateAll() progress events = %d, want 2 (one per role, one per file)", events)
	}
	if errs[0] != nil {
		t.Errorf("UpdateAll() errs[0] = %v, want nil", errs[0])
	}
	if errs[1] == nil || !strings.Contains(errs[1].Error(), "writing file") {
		t.Fatalf("UpdateAll() errs[1] = %v, want a writing file error", errs[1])
	}
}
