package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
)

// NewProgressbar creates a new progress bar with the specified length and description
func NewProgressbar(length int, description string) *progressbar.ProgressBar {
	return progressbar.NewOptions(length,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetMaxDetailRow(10),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetRenderBlankState(true),
	)
}

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
