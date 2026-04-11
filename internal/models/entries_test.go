package models

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gopkg.in/yaml.v3"
)

func TestGetName(t *testing.T) {
	tests := []struct {
		name     string
		entry    Entry
		expected string
	}{
		{
			name:     "explicit Name field takes priority",
			entry:    Entry{Src: "git+https://github.com/org/other-role.git", Name: "my-role"},
			expected: "my-role",
		},
		{
			name:     "name derived from Src basename without .git",
			entry:    Entry{Src: "git+https://github.com/org/ansible-role-nginx.git"},
			expected: "ansible-role-nginx",
		},
		{
			name:     "name derived from Src without .git suffix",
			entry:    Entry{Src: "https://github.com/org/my-role"},
			expected: "my-role",
		},
		{
			name:     "cached name returned on second call",
			entry:    Entry{Src: "git+https://github.com/org/role.git"},
			expected: "role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.GetName()
			if got != tt.expected {
				t.Errorf("GetName() = %q, want %q", got, tt.expected)
			}
			// Second call should return cached value
			if got2 := tt.entry.GetName(); got2 != tt.expected {
				t.Errorf("GetName() second call = %q, want %q", got2, tt.expected)
			}
		})
	}
}

func TestGetPath(t *testing.T) {
	entry := Entry{Name: "my-role"}
	got := entry.GetPath("/roles/galaxy")
	expected := "/roles/galaxy/my-role"
	if got != expected {
		t.Errorf("GetPath() = %q, want %q", got, expected)
	}
}

func TestGetInstallInfoPath(t *testing.T) {
	entry := Entry{Name: "my-role"}
	got := entry.GetInstallInfoPath("/roles/galaxy")
	expected := "/roles/galaxy/my-role/meta/.galaxy_install_info"
	if got != expected {
		t.Errorf("GetInstallInfoPath() = %q, want %q", got, expected)
	}
}

func TestGenerateInstallInfo(t *testing.T) {
	entry := Entry{Name: "my-role", Version: "v1.2.3"}
	commitSHA := "abc123def456"

	outb, err := entry.GenerateInstallInfo(commitSHA)
	if err != nil {
		t.Fatalf("GenerateInstallInfo() error = %v", err)
	}

	var info GalaxyInstallInfo
	if err := yaml.Unmarshal(outb, &info); err != nil {
		t.Fatalf("failed to unmarshal generated info: %v", err)
	}

	if info.Version != "v1.2.3" {
		t.Errorf("Version = %q, want %q", info.Version, "v1.2.3")
	}
	if info.InstallCommit != commitSHA {
		t.Errorf("InstallCommit = %q, want %q", info.InstallCommit, commitSHA)
	}
	// Ansible-galaxy trailing space is preserved
	if !strings.HasSuffix(info.InstallDate, " ") {
		t.Errorf("InstallDate %q should have trailing space", info.InstallDate)
	}
	// Date must be parseable
	_, err = time.Parse("Mon 02 Jan 2006 03:04:05 PM ", info.InstallDate)
	if err != nil {
		t.Errorf("InstallDate %q not parseable: %v", info.InstallDate, err)
	}
}

func TestGetInstallInfo(t *testing.T) {
	infoYAML := "install_date: \"Mon 01 Jan 2024 12:00:00 PM \"\ninstall_commit: deadbeef\nversion: v2.0.0\n"

	fsys := fstest.MapFS{
		"my-role/meta/.galaxy_install_info": &fstest.MapFile{
			Data: []byte(infoYAML),
		},
	}

	t.Run("reads install info from FS", func(t *testing.T) {
		entry := Entry{Name: "my-role"}
		info, err := entry.GetInstallInfo(fsys)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Version != "v2.0.0" {
			t.Errorf("Version = %q, want %q", info.Version, "v2.0.0")
		}
		if info.InstallCommit != "deadbeef" {
			t.Errorf("InstallCommit = %q, want %q", info.InstallCommit, "deadbeef")
		}
	})

	t.Run("returns zero value when file missing", func(t *testing.T) {
		entry := Entry{Name: "nonexistent-role"}
		info, err := entry.GetInstallInfo(fsys)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Version != "" {
			t.Errorf("Version = %q, want empty string for missing role", info.Version)
		}
	})
}

func TestIsInstalled(t *testing.T) {
	makeFS := func(roleName, version string) fstest.MapFS {
		return fstest.MapFS{
			roleName: &fstest.MapFile{Mode: 0o700 | 0o040000000},
			roleName + "/meta/.galaxy_install_info": &fstest.MapFile{
				Data: []byte("version: " + version + "\n"),
			},
		}
	}

	t.Run("not installed when role dir missing", func(t *testing.T) {
		entry := Entry{Name: "missing-role", Version: "v1.0.0"}
		if entry.IsInstalled(fstest.MapFS{}) {
			t.Error("IsInstalled() = true, want false for missing role dir")
		}
	})

	t.Run("not installed when version mismatch", func(t *testing.T) {
		entry := Entry{Name: "my-role", Version: "v2.0.0"}
		if entry.IsInstalled(makeFS("my-role", "v1.0.0")) {
			t.Error("IsInstalled() = true, want false for version mismatch")
		}
	})

	t.Run("not installed for forced versions (main)", func(t *testing.T) {
		entry := Entry{Name: "my-role", Version: "main"}
		if entry.IsInstalled(makeFS("my-role", "main")) {
			t.Error("IsInstalled() = true, want false for forced version main")
		}
	})

	t.Run("not installed for forced versions (master)", func(t *testing.T) {
		entry := Entry{Name: "my-role", Version: "master"}
		if entry.IsInstalled(makeFS("my-role", "master")) {
			t.Error("IsInstalled() = true, want false for forced version master")
		}
	})

	t.Run("installed when dir exists and version matches", func(t *testing.T) {
		entry := Entry{Name: "my-role", Version: "v1.0.0"}
		if !entry.IsInstalled(makeFS("my-role", "v1.0.0")) {
			t.Error("IsInstalled() = false, want true for installed role")
		}
	})
}
