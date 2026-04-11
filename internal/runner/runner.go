package runner

import (
	"os/exec"
	"strings"
)

// Runner is an interface for executing shell commands.
// Implementations are expected to return combined stdout+stderr output.
type Runner interface {
	Run(command, dir string) (string, error)
}

// ShellRunner executes shell commands via os/exec.
// It implements the Runner interface using the system shell.
type ShellRunner struct{}

// New creates a new ShellRunner
func New() *ShellRunner {
	return &ShellRunner{}
}

// Run executes a shell command in the given directory and returns combined output
func (r *ShellRunner) Run(command, dir string) (string, error) {
	slice := strings.Split(command, " ")
	cmd := exec.Command(slice[0], slice[1:]...) //nolint:gosec // that's intended
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if out == nil {
		return "", err
	}

	return strings.TrimSuffix(string(out), "\n"), err
}
