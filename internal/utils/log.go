package utils

import "fmt"

// Verbose is a flag to enable verbose logging
var Verbose bool

func Log(v ...any) {
	v = append([]any{"[a.g.r.u]"}, v...)
	fmt.Println(v...)
}

func Debug(v ...any) {
	if Verbose {
		Log(v...)
	}
}
