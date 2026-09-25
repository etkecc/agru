//go:build e2e

package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// preCommit is the consumer that drives agru through pre-commit's hook machinery from this repo's hooks file.
var preCommit = &caller{
	repo:   "pre-commit",
	recipe: "hook: agru, pre-commit run --all-files",
	source: "https://github.com/etkecc/agru/blob/HEAD/.pre-commit-hooks.yaml",
	argv:   []string{},
}

// preCommitPinned is the hook consumer's requirements.yml: one tag pin staged a release behind upstream.
const preCommitPinned = `---

- src: git+https://github.com/mother-of-all-self-hosting/ansible-role-cleanup.git
  version: v1.0.0-1
  name: cleanup
`

// TestPreCommitHook pins the hook contract: the hook bumps the lagging pin, installs it, then passes clean on re-run.
func TestPreCommitHook(t *testing.T) {
	if _, err := exec.LookPath("pre-commit"); err != nil {
		t.Skip("pre-commit is not installed; the hook contract is covered by a manual run")
	}
	p := newProject(t, preCommit, map[string]string{
		"requirements.yml": preCommitPinned,
		".pre-commit-config.yaml": fmt.Sprintf(`repos:
  - repo: file://%s
    rev: HEAD
    hooks:
      - id: agru
`, repoRoot()),
	})
	p.gitInit()
	before := p.read("requirements.yml")
	bumped := newestTag(t, etkeccCleanupSrc)
	if bumped == etkeccCleanupPin {
		t.Fatalf("fixture is no longer behind: pin %s at an older tag of %s", etkeccCleanupPin, etkeccCleanupSrc)
	}
	// First run: the hook bumps the pin and installs, pre-commit fails with the modified-files report.
	r := p.runPreCommit()
	if r.code != 1 {
		p.t.Fatalf("%s: pre-commit exited %d, want 1 (files modified by this hook)\n%s", p.caller.label(), r.code, r.output())
	}
	p.requireTextEqual("requirements.yml", p.read("requirements.yml"), replacePin(before, etkeccCleanupPin, bumped))
	p.requirePreCommitInstalled("cleanup", bumped)
	// Second run: nothing left to do, a clean pass with no rewrites.
	r = p.runPreCommit()
	if r.code != 0 {
		p.t.Fatalf("%s: re-run pre-commit exited %d, want 0\n%s", p.caller.label(), r.code, r.output())
	}
	p.requireTextEqual("requirements.yml", p.read("requirements.yml"), replacePin(before, etkeccCleanupPin, bumped))
}

// gitInit makes the scratch checkout a repo, so pre-commit can enumerate its files.
func (p *project) gitInit() {
	p.t.Helper()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "e2e@example.com"},
		{"config", "user.name", "agru e2e"},
		{"add", "-A"},
		{"commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = p.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			p.t.Fatalf("%s: git %s: %v\n%s", p.caller.label(), strings.Join(args, " "), err, out)
		}
	}
}

// runPreCommit drives the consumer's pre-commit from the checkout, with a scratch HOME so its caches stay local.
func (p *project) runPreCommit() runResult {
	p.t.Helper()
	cmd := exec.Command("pre-commit", "run", "agru", "--all-files")
	cmd.Dir = p.dir
	cmd.Env = append(scratchEnv(), "HOME="+p.path("home"), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		p.t.Fatalf("%s: running pre-commit: %v\n%s", p.caller.label(), err, stdout.String()+stderr.String())
	}
	if exitErr != nil {
		code = exitErr.ExitCode()
	}
	p.last = runResult{args: cmd.Args, code: code, stdout: stdout.String(), stderr: stderr.String()}
	return p.last
}

// requirePreCommitInstalled asserts the role landed in the HOME roles path at the pinned version.
func (p *project) requirePreCommitInstalled(role, wantVersion string) {
	p.t.Helper()
	rel := "home/.ansible/roles/" + role + "/meta/.galaxy_install_info"
	var info installInfo
	if err := yaml.Unmarshal([]byte(p.read(rel)), &info); err != nil {
		p.t.Fatalf("%s: %s was not installed: %v", p.caller.label(), rel, err)
	}
	if info.Version != wantVersion {
		p.t.Errorf("%s: %s install info version = %q, want %q (the bumped version)", p.caller.label(), rel, info.Version, wantVersion)
	}
}
