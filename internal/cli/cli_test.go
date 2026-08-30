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

// fakeRunner records commands and returns preset outputs matched by prefix.
type fakeRunner struct {
	mu      sync.Mutex
	outputs map[string]string
	calls   []string
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: make(map[string]string)}
}

func (r *fakeRunner) Run(command, _ string) (string, error) {
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

// each mode's Run must reach exactly the service the flag selects. we watch the git
// commands the fake records, since that is the observable proof the branch fired.

func TestRunListTouchesNoGit(t *testing.T) {
	dir := t.TempDir()
	fr := newFakeRunner()
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: dir, ListInstalled: true}

	if err := Run(cfg, parser.New(fr), installer.New(fr, dir, 0, true)); err != nil {
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

	_ = Run(cfg, parser.New(fr), installer.New(fr, filepath.Join(dir, "roles"), 0, true))
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

	if err := Run(cfg, parser.New(fr), installer.New(fr, dir, 0, true)); err != nil {
		t.Fatalf("Run() update error = %v", err)
	}
	if !fr.called("git ls-remote") {
		t.Errorf("update mode did not check remote: %v", fr.calls)
	}
}

// -u -i is fail-fast by design: a failed update must not fall through to install, or
// we'd deploy bumped versions that never reached requirements.yml. this pins that,
// against the TUI which installs anyway.
func TestRunUpdateInstallFailFast(t *testing.T) {
	dir := t.TempDir()
	fr := newFakeRunner()
	fr.outputs["git ls-remote"] = "garbage-without-any-tag-ref\n" // getNewVersion can't parse a tag, so the update errors
	roles := filepath.Join(dir, "roles")
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: roles, UpdateFile: true, InstallMissing: true}

	if err := Run(cfg, parser.New(fr), installer.New(fr, roles, 0, true)); err == nil {
		t.Fatal("Run() -u -i with a failing update: want error, got nil")
	}
	if fr.called("git clone") {
		t.Errorf("fail-fast violated: install ran after the update errored: %v", fr.calls)
	}
}

func TestRunDeleteRemovesDir(t *testing.T) {
	dir := t.TempDir()
	roleDir := filepath.Join(dir, "role-a")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatalf("seeding role dir: %v", err)
	}
	fr := newFakeRunner()
	cfg := config.Config{RequirementsPath: writeReqs(t, dir), RolesPath: dir, DeleteName: "role-a"}

	if err := Run(cfg, parser.New(fr), installer.New(fr, dir, 0, true)); err != nil {
		t.Fatalf("Run() delete error = %v", err)
	}
	if _, err := os.Stat(roleDir); !os.IsNotExist(err) {
		t.Errorf("delete mode left role dir on disk: stat err = %v", err)
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
	if err := Run(cfg, parser.New(fr), installer.New(fr, dir, 0, true)); err != nil {
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
	if err := Run(cfg, parser.New(newFakeRunner()), installer.New(newFakeRunner(), dir, 0, true)); err == nil {
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

	err := runUpdate(cfg, parser.New(fr), []parser.RequirementsFile{{Path: bad, Entries: entries()}})
	if err == nil || !strings.HasPrefix(err.Error(), "writing file "+bad+":") {
		t.Fatalf("runUpdate() single file = %v, want the unprefixed writing file error", err)
	}

	good := filepath.Join(t.TempDir(), "ok.yml")
	err = runUpdate(cfg, parser.New(fr), []parser.RequirementsFile{{Path: good, Entries: entries()}, {Path: bad, Entries: entries()}})
	if err == nil || !strings.Contains(err.Error(), bad+": writing file") {
		t.Fatalf("runUpdate() multi-file = %v, want the path-prefixed writing file error", err)
	}
}
