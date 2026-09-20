package runner

import (
	"os/exec"
	"strings"
)

// Runner executes commands from an argv slice, returning combined stdout+stderr output.
type Runner interface {
	// RunArgs executes a command from an argv slice, bypassing shell splitting.
	RunArgs(args []string, dir string) (string, error)
}

// ShellRunner implements Runner via os/exec against the system shell.
type ShellRunner struct{}

// New creates a new ShellRunner
func New() *ShellRunner {
	return &ShellRunner{}
}

// RunArgs executes a command from an argv slice directly, no shell splitting.
func (r *ShellRunner) RunArgs(args []string, dir string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // caller must validate args
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if out == nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), err
}
