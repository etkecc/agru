package runner

import (
	"os/exec"
	"strings"
)

// Runner executes shell commands, returning combined stdout+stderr output.
type Runner interface {
	Run(command, dir string) (string, error)
	// RunArgs executes a command from an argv slice, bypassing shell splitting.
	RunArgs(args []string, dir string) (string, error)
}

// ShellRunner implements Runner via os/exec against the system shell.
type ShellRunner struct{}

// New creates a new ShellRunner
func New() *ShellRunner {
	return &ShellRunner{}
}

// Run splits a command string on spaces; safe only for trusted input.
func (r *ShellRunner) Run(command, dir string) (string, error) {
	slice := strings.Split(command, " ")
	return r.RunArgs(slice, dir)
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
