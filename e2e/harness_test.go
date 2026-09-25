//go:build e2e

// Package e2e emulates agru's consumers: the real binary, their argv, real roles over the network.
package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// agruPath is the binary built from this working tree, shared by every case.
var agruPath string

// TestMain builds agru once, so cases exercise the code under review and not a released binary.
func TestMain(m *testing.M) {
	bin, err := build()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	agruPath = bin
	code := m.Run()
	os.RemoveAll(filepath.Dir(bin))
	os.Exit(code)
}

// build compiles cmd/agru into a temp dir and returns the binary path.
func build() (string, error) {
	dir, err := os.MkdirTemp("", "agru-e2e-")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	bin := filepath.Join(dir, "agru")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/agru")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building agru: %w\n%s", err, out)
	}
	return bin, nil
}

// repoRoot is this checkout's root, one level above the package.
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("e2e: cannot locate the harness file")
	}
	return filepath.Dir(filepath.Dir(file))
}

// caller is one agru invocation the suite emulates: which repo, which recipe, which argv.
type caller struct {
	repo   string   // the consumer
	recipe string   // the recipe or workflow step that calls agru
	source string   // permalink, so a red run points at the code being emulated
	argv   []string // argv the recipe builds; {{dir}} becomes the scratch checkout path
	drift  []check  // upstream snippets this emulation is copied from
}

// check is one upstream file plus the snippets the emulation depends on.
type check struct {
	url  string
	want []string
}

// label names the caller in failure messages.
func (c *caller) label() string {
	return c.repo + " " + c.recipe + " " + c.source
}

// runResult is one agru invocation: the argv it ran, its exit code, and its output.
type runResult struct {
	args   []string
	code   int
	stdout string
	stderr string
}

// output is everything agru printed, wherever it printed it.
func (r runResult) output() string {
	return r.stdout + r.stderr
}

// project is a scratch checkout standing in for one caller's working tree.
type project struct {
	t      *testing.T
	caller caller
	dir    string
	last   runResult
}

// newProject creates the checkout and drops the caller's requirements files into it.
func newProject(t *testing.T, c *caller, files map[string]string) *project {
	t.Helper()
	p := &project{t: t, caller: *c, dir: t.TempDir()}
	if err := os.MkdirAll(p.path("home"), 0o755); err != nil {
		t.Fatalf("%s: creating scratch home: %v", c.label(), err)
	}
	for rel, content := range files {
		p.write(rel, content)
	}
	return p
}

// path resolves a slash-separated path inside the scratch checkout.
func (p *project) path(rel string) string {
	return filepath.Join(p.dir, filepath.FromSlash(rel))
}

// write drops a file into the checkout, creating parents.
func (p *project) write(rel, content string) {
	p.t.Helper()
	full := p.path(rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		p.t.Fatalf("%s: creating %s: %v", p.caller.label(), filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		p.t.Fatalf("%s: writing %s: %v", p.caller.label(), rel, err)
	}
}

// read returns a file from the checkout.
func (p *project) read(rel string) string {
	p.t.Helper()
	content, err := os.ReadFile(p.path(rel))
	if err != nil {
		p.t.Fatalf("%s: reading %s: %v", p.caller.label(), rel, err)
	}
	return string(content)
}

// run drives the recipe's argv from the checkout, with a scratch HOME and no interactive prompts.
func (p *project) run(extra ...string) runResult {
	p.t.Helper()
	args := make([]string, 0, len(p.caller.argv)+len(extra))
	for _, arg := range p.caller.argv {
		args = append(args, strings.ReplaceAll(arg, "{{dir}}", p.dir))
	}
	args = append(args, extra...)
	cmd := exec.Command(agruPath, args...)
	cmd.Dir = p.dir
	cmd.Env = append(scratchEnv(), "HOME="+p.path("home"), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		p.t.Fatalf("%s: running agru %s: %v", p.caller.label(), strings.Join(args, " "), err)
	}
	if exitErr != nil {
		code = exitErr.ExitCode()
	}
	p.last = runResult{args: args, code: code, stdout: stdout.String(), stderr: stderr.String()}
	return p.last
}

// scratchEnv is the ambient env minus anything that would make cases inherit a user's agru setup.
func scratchEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AGRU_") || strings.HasPrefix(kv, "ANSIBLE_") || strings.HasPrefix(kv, "HOME=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// requireOK fails when the recipe's command did not exit 0, and prints what agru said.
func (p *project) requireOK(r runResult) {
	p.t.Helper()
	if r.code == 0 {
		return
	}
	p.t.Fatalf("%s: agru %s exited %d, want 0\n%s", p.caller.label(), strings.Join(r.args, " "), r.code, r.output())
}
