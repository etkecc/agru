package utils

import (
	"os/exec"
	"strings"
)

// Run executes a command in a given directory
func Run(command, dir string) (string, error) {
	slice := strings.Split(command, " ")
	cmd := exec.Command(slice[0], slice[1:]...) //nolint:gosec // that's intended
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	Debug("execute")
	Debug("    command:", command)
	Debug("    chdir:", dir)
	if out != nil {
		Debug("    output:", strings.TrimSuffix(string(out), "\n"))
	}
	if out == nil {
		return "", err
	}

	return strings.TrimSuffix(string(out), "\n"), err
}
