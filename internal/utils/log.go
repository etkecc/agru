package utils

import (
	"fmt"
	"os"
)

// Log prints a message with the [a.g.r.u] prefix
func Log(v ...any) {
	v = append([]any{"[a.g.r.u]"}, v...)
	fmt.Println(v...)
}

// Error goes to stderr, not stdout, so redirecting a run to a file doesn't swallow the thing that went wrong.
func Error(v ...any) {
	v = append([]any{"[a.g.r.u]"}, v...)
	fmt.Fprintln(os.Stderr, v...)
}

// Debug prints a message only when verbose is true
func Debug(verbose bool, v ...any) {
	if verbose {
		Log(v...)
	}
}
