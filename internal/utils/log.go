package utils

import "fmt"

// Log prints a message with the [a.g.r.u] prefix
func Log(v ...any) {
	v = append([]any{"[a.g.r.u]"}, v...)
	fmt.Println(v...)
}

// Debug prints a message only when verbose is true
func Debug(verbose bool, v ...any) {
	if verbose {
		Log(v...)
	}
}
