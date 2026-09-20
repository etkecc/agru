package runner

import (
	"testing"
)

func TestShellRunnerRunArgs(t *testing.T) {
	r := New()

	t.Run("returns stdout output", func(t *testing.T) {
		out, err := r.RunArgs([]string{"echo", "hello"}, "")
		if err != nil {
			t.Fatalf("RunArgs() error = %v", err)
		}
		if out != "hello" {
			t.Errorf("RunArgs() = %q, want %q", out, "hello")
		}
	})

	t.Run("returns error for failing command", func(t *testing.T) {
		_, err := r.RunArgs([]string{"false"}, "")
		if err == nil {
			t.Error("RunArgs() expected error for 'false' command, got nil")
		}
	})

	t.Run("runs in specified directory", func(t *testing.T) {
		out, err := r.RunArgs([]string{"pwd"}, "/tmp")
		if err != nil {
			t.Fatalf("RunArgs() error = %v", err)
		}
		if out != "/tmp" {
			t.Errorf("RunArgs() pwd = %q, want /tmp", out)
		}
	})

	t.Run("trims trailing newline from output", func(t *testing.T) {
		out, err := r.RunArgs([]string{"printf", "hello"}, "")
		if err != nil {
			t.Fatalf("RunArgs() error = %v", err)
		}
		if out != "hello" {
			t.Errorf("RunArgs() = %q, want %q", out, "hello")
		}
	})
}
