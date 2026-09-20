package models

import (
	"testing"
)

func TestValidateGitArg(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantErr bool
	}{
		{name: "git url", arg: "https://x/y.git"},
		{name: "tag version", arg: "v1.0.0"},
		{name: "plain string", arg: "abc123"},
		{name: "empty", arg: "", wantErr: true},
		{name: "leading dash", arg: "-x", wantErr: true},
		{name: "space", arg: "a b", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGitArg(tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGitArg(%q) error = %v, wantErr %v", tt.arg, err, tt.wantErr)
			}
		})
	}
}
