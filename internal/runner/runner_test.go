package runner

import (
	"testing"
)

func TestShellRunnerRun(t *testing.T) {
	r := New(false)

	t.Run("returns stdout output", func(t *testing.T) {
		out, err := r.Run("echo hello", "")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if out != "hello" {
			t.Errorf("Run() = %q, want %q", out, "hello")
		}
	})

	t.Run("returns error for failing command", func(t *testing.T) {
		_, err := r.Run("false", "")
		if err == nil {
			t.Error("Run() expected error for 'false' command, got nil")
		}
	})

	t.Run("runs in specified directory", func(t *testing.T) {
		out, err := r.Run("pwd", "/tmp")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if out != "/tmp" {
			t.Errorf("Run() pwd = %q, want /tmp", out)
		}
	})

	t.Run("trims trailing newline from output", func(t *testing.T) {
		out, err := r.Run("printf hello", "")
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if out != "hello" {
			t.Errorf("Run() = %q, want %q", out, "hello")
		}
	})
}
