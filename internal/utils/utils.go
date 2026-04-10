package utils

import (
	"fmt"
	"os"
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
