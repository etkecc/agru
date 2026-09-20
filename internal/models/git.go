package models

import (
	"fmt"
	"strings"
)

// ValidateGitArg rejects strings that could inject git options (spaces, leading dash).
func ValidateGitArg(s string) error {
	if s == "" {
		return fmt.Errorf("empty argument")
	}
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("must not start with dash (possible option injection)")
	}
	if strings.Contains(s, " ") {
		return fmt.Errorf("must not contain spaces (possible option injection)")
	}
	return nil
}
