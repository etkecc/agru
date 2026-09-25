package cli

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/etkecc/agru/internal/config"
	"github.com/etkecc/agru/internal/installer"
	"github.com/etkecc/agru/internal/models"
	"github.com/etkecc/agru/internal/parser"
)

// seedManifest is a minimal installed-collection MANIFEST.json for delete fixtures.
const seedManifest = `{"collection_info":{"namespace":"community","name":"docker","version":"3.13.0"},"format":"1.0.0"}`

// fakeRunner records commands and returns preset outputs matched by prefix.
type fakeRunner struct {
	mu      sync.Mutex
	outputs map[string]string
	calls   []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: make(map[string]string)}
}

func (r *fakeRunner) run(command, _ string) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, command)
	r.mu.Unlock()
	for key, out := range r.outputs {
		if strings.HasPrefix(command, key) {
			return out, nil
		}
	}
	return "", nil
}

func (r *fakeRunner) RunArgs(args []string, dir string) (string, error) {
	return r.run(strings.Join(args, " "), dir)
}

func (r *fakeRunner) called(prefix string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func writeReqs(t *testing.T, dir string) string {
	t.Helper()
	fp := filepath.Join(dir, "requirements.yml")
	content := "- src: git+https://github.com/org/role-a.git\n  version: v1.0.0\n"
	if err := os.WriteFile(fp, []byte(content), 0o600); err != nil {
		t.Fatalf("writing requirements: %v", err)
	}
	return fp
}

// each mode's Run must reach exactly the service the flag selects, proven via the git commands the fake records.

func TestRunListTouchesNoGit(t *testing.T) {
	dir := t.TempDir()
	fr := newFakeRunner()
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: dir, ListInstalled: true}

	if err := Run(&cfg, parser.New(fr), installer.New(fr, dir, "", 0, true)); err != nil {
		t.Fatalf("Run() list error = %v", err)
	}
	if fr.called("git") {
		t.Errorf("list mode ran git: %v", fr.calls)
	}
}

func TestRunInstallClones(t *testing.T) {
	dir := t.TempDir()
	fr := newFakeRunner()
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: filepath.Join(dir, "roles"), InstallMissing: true}

	_ = Run(&cfg, parser.New(fr), installer.New(fr, filepath.Join(dir, "roles"), "", 0, true))
	if !fr.called("git clone") {
		t.Errorf("install mode did not clone: %v", fr.calls)
	}
}

func TestRunUpdateChecksRemote(t *testing.T) {
	dir := t.TempDir()
	fr := newFakeRunner()
	fr.outputs["git ls-remote"] = "abc123\trefs/tags/v2.0.0\n"
	reqs := writeReqs(t, dir)
	cfg := config.Config{RequirementsPath: reqs, RolesPath: dir, UpdateFile: true}

	if err := Run(&cfg, parser.New(fr), installer.New(fr, dir, "", 0, true)); err != nil {
		t.Fatalf("Run() update error = %v", err)
	}
	if !fr.called("git ls-remote") {
		t.Errorf("update mode did not check remote: %v", fr.calls)
	}
}

// -u -i is fail-fast by design: a failed update must not fall through to install and deploy an unpinned version.
func TestRunUpdateInstallFailFast(t *testing.T) {
	dir := t.TempDir()
	fr := newFakeRunner()
	fr.outputs["git ls-remote"] = "garbage-without-any-tag-ref\n" // getNewVersion can't parse a tag, so the update errors
	roles := filepath.Join(dir, "roles")
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: roles, UpdateFile: true, InstallMissing: true}

	if err := Run(&cfg, parser.New(fr), installer.New(fr, roles, "", 0, true)); err == nil {
		t.Fatal("Run() -u -i with a failing update: want error, got nil")
	}
	if fr.called("git clone") {
		t.Errorf("fail-fast violated: install ran after the update errored: %v", fr.calls)
	}
}

func TestRunDeleteRemovesDir(t *testing.T) {
	dir := t.TempDir()
	roleDir := filepath.Join(dir, "role-a")
	if err := os.MkdirAll(filepath.Join(roleDir, "meta"), 0o755); err != nil {
		t.Fatalf("seeding role dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "meta", "main.yml"), []byte("galaxy_info:\n"), 0o600); err != nil {
		t.Fatalf("seeding role meta: %v", err)
	}
	fr := newFakeRunner()
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: dir, DeleteName: "role-a"}

	if err := Run(&cfg, parser.New(fr), installer.New(fr, dir, "", 0, true)); err != nil {
		t.Fatalf("Run() delete error = %v", err)
	}
	if _, err := os.Stat(roleDir); !os.IsNotExist(err) {
		t.Errorf("delete mode left role dir on disk: stat err = %v", err)
	}
}

// -d must refuse a directory that is not a role and leave it alone.
func TestRunDeleteRefusesNonRoleDir(t *testing.T) {
	dir := t.TempDir()
	decoy := filepath.Join(dir, "role-a")
	if err := os.MkdirAll(filepath.Join(decoy, "sub"), 0o755); err != nil {
		t.Fatalf("seeding decoy dir: %v", err)
	}
	fr := newFakeRunner()
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: dir, DeleteName: "role-a"}

	if err := Run(&cfg, parser.New(fr), installer.New(fr, dir, "", 0, true)); err == nil {
		t.Fatal("Run() delete of a non-role dir: want error, got nil")
	}
	if _, err := os.Stat(filepath.Join(decoy, "sub")); err != nil {
		t.Errorf("delete refused but removed the dir anyway: stat err = %v", err)
	}
}

// -r glob: one update run must rewrite every matched file in place.
func TestRunUpdateMultiFile(t *testing.T) {
	fr := newFakeRunner()
	repoA := "https://github.com/org/role-a.git"
	repoB := "https://github.com/org/role-b.git"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repoA] = "abc\trefs/tags/v2.0.0"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repoB] = "def\trefs/tags/v3.0.0"

	dir := t.TempDir()
	write := func(sub, repo string) {
		fp := filepath.Join(dir, sub, "requirements.yml")
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", sub, err)
		}
		if err := os.WriteFile(fp, []byte("- src: git+"+repo+"\n  version: v1.0.0\n"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", fp, err)
		}
	}
	write("mariadb", repoA)
	write("postgres", repoB)

	cfg := config.Config{RequirementsPath: filepath.Join(dir, "*", "requirements.yml"), RolesPath: dir, UpdateFile: true}
	if err := Run(&cfg, parser.New(fr), installer.New(fr, dir, "", 0, true)); err != nil {
		t.Fatalf("Run() multi-file update error = %v", err)
	}
	for sub, want := range map[string]string{"mariadb": "v2.0.0", "postgres": "v3.0.0"} {
		out, err := os.ReadFile(filepath.Join(dir, sub, "requirements.yml"))
		if err != nil {
			t.Fatalf("reading %s: %v", sub, err)
		}
		if !strings.Contains(string(out), want) {
			t.Errorf("Run() %s/requirements.yml not updated to %s:\n%s", sub, want, out)
		}
	}
}

// a non-matching -r pattern fails the run with the no-match error.
func TestRunNoMatchingPattern(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{RequirementsPath: filepath.Join(dir, "nothing", "*.yml"), RolesPath: dir, UpdateFile: true}
	if err := Run(&cfg, parser.New(newFakeRunner()), installer.New(newFakeRunner(), dir, "", 0, true)); err == nil {
		t.Fatal("Run() with a non-matching -r pattern: want error, got nil")
	}
}

// write failures keep the original wording for a single file, and carry the path prefix for several.
func TestRunUpdateWriteErrorWording(t *testing.T) {
	fr := newFakeRunner()
	repo := "https://github.com/org/role-a.git"
	fr.outputs["git ls-remote -tq --sort=-version:refname "+repo] = "abc\trefs/tags/v2.0.0"
	entries := func() models.File { return models.File{{Name: "role-a", Src: "git+" + repo, Version: "v1.0.0"}} }

	bad := t.TempDir() // a directory: the write must fail even as root
	cfg := config.Config{RolesPath: bad}

	err := runUpdate(&cfg, parser.New(fr), []parser.RequirementsFile{{Path: bad, Entries: entries()}})
	if err == nil || !strings.HasPrefix(err.Error(), "writing file "+bad+":") {
		t.Fatalf("runUpdate() single file = %v, want the unprefixed writing file error", err)
	}

	good := filepath.Join(t.TempDir(), "ok.yml")
	err = runUpdate(&cfg, parser.New(fr), []parser.RequirementsFile{{Path: good, Entries: entries()}, {Path: bad, Entries: entries()}})
	if err == nil || !strings.Contains(err.Error(), bad+": writing file") {
		t.Fatalf("runUpdate() multi-file = %v, want the path-prefixed writing file error", err)
	}
}

// Install with a mix of supported and unsupported collections.
func TestRunInstallWithCollections(t *testing.T) {
	dir := t.TempDir()
	fr := newFakeRunner()
	fr.outputs["git clone"] = ""

	// Write a requirements file with both roles and collections
	fp := filepath.Join(dir, "requirements.yml")
	content := `---
collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0
  - community.general

roles:
  - src: git+https://github.com/org/role-a.git
    version: v1.0.0
`
	if err := os.WriteFile(fp, []byte(content), 0o600); err != nil {
		t.Fatalf("writing requirements: %v", err)
	}

	cfg := config.Config{
		RequirementsPath: fp,
		RolesPath:        filepath.Join(dir, "roles"),
		CollectionsPath:  filepath.Join(dir, "collections"),
		InstallMissing:   true,
		NoTUI:            true,
	}

	_ = Run(&cfg, parser.New(fr), installer.New(fr, filepath.Join(dir, "roles"), filepath.Join(dir, "collections"), 0, true))
	if !fr.called("git clone") {
		t.Errorf("install mode should have called git clone, calls: %v", fr.calls)
	}
}

// Delete a collection by FQCN
func TestRunDeleteCollection(t *testing.T) {
	dir := t.TempDir()
	collDir := filepath.Join(dir, "collections", "ansible_collections", "community", "docker")
	if err := os.MkdirAll(collDir, 0o755); err != nil {
		t.Fatalf("seeding collection dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(collDir, "MANIFEST.json"), []byte(seedManifest), 0o600); err != nil {
		t.Fatalf("seeding collection manifest: %v", err)
	}

	fr := newFakeRunner()
	cfg := config.Config{
		RequirementsPath: writeReqs(t, dir),
		RolesPath:        dir,
		CollectionsPath:  filepath.Join(dir, "collections"),
		DeleteName:       "community.docker",
	}

	// Add collection to the file so it gets picked up
	fp := filepath.Join(dir, "requirements.yml")
	content := `---
collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0

roles:
  - src: git+https://github.com/org/role-a.git
    version: v1.0.0
`
	if err := os.WriteFile(fp, []byte(content), 0o600); err != nil {
		t.Fatalf("writing requirements: %v", err)
	}

	if err := Run(&cfg, parser.New(fr), installer.New(fr, dir, filepath.Join(dir, "collections"), 0, true)); err != nil {
		t.Fatalf("Run() delete error = %v", err)
	}
	if _, err := os.Stat(collDir); !os.IsNotExist(err) {
		t.Errorf("delete mode left collection dir on disk: stat err = %v", err)
	}
}

// -d must refuse a dir under the collections path that is not an installed collection.
func TestRunDeleteRefusesNonCollectionDir(t *testing.T) {
	dir := t.TempDir()
	collDir := filepath.Join(dir, "collections", "ansible_collections", "community", "docker")
	if err := os.MkdirAll(filepath.Join(collDir, "plugins"), 0o755); err != nil {
		t.Fatalf("seeding decoy dir: %v", err)
	}
	fr := newFakeRunner()
	cfg := config.Config{
		RequirementsPath: writeReqs(t, dir),
		RolesPath:        dir,
		CollectionsPath:  filepath.Join(dir, "collections"),
		DeleteName:       "community.docker",
	}
	fp := filepath.Join(dir, "requirements.yml")
	content := `---
collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0
`
	if err := os.WriteFile(fp, []byte(content), 0o600); err != nil {
		t.Fatalf("writing requirements: %v", err)
	}

	if err := Run(&cfg, parser.New(fr), installer.New(fr, dir, filepath.Join(dir, "collections"), 0, true)); err == nil {
		t.Fatal("Run() delete of a non-collection dir: want error, got nil")
	}
	if _, err := os.Stat(filepath.Join(collDir, "plugins")); err != nil {
		t.Errorf("delete refused but removed the dir anyway: stat err = %v", err)
	}
}
