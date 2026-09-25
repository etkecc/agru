package pathfinder

import (
	"path/filepath"
	"testing"

	"github.com/etkecc/agru/internal/config"
)

// clearPathEnv unsets every ansible path env var so a test starts from a known state.
func clearPathEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ANSIBLE_ROLES_PATH", "")
	t.Setenv("ANSIBLE_COLLECTIONS_PATH", "")
	t.Setenv("ANSIBLE_COLLECTIONS_PATHS", "")
	t.Setenv("ANSIBLE_HOME", "")
}

// resolvedPaths resolves install paths for the given -p and -cp flag values.
func resolvedPaths(t *testing.T, rolesFlag, collectionsFlag string) config.Config {
	t.Helper()
	cfg := config.Config{RolesPath: rolesFlag, CollectionsPath: collectionsFlag}
	if err := ResolveInstallPaths(&cfg); err != nil {
		t.Fatalf("ResolveInstallPaths error: %v", err)
	}
	return cfg
}

func TestInstallPathDefaults(t *testing.T) {
	clearPathEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := resolvedPaths(t, "", "")
	if want := filepath.Join(home, ".ansible", "roles"); cfg.RolesPath != want {
		t.Errorf("RolesPath = %q, want %q", cfg.RolesPath, want)
	}
	if want := filepath.Join(home, ".ansible", "collections"); cfg.CollectionsPath != want {
		t.Errorf("CollectionsPath = %q, want %q", cfg.CollectionsPath, want)
	}
}

func TestInstallPathNoHome(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("HOME", "")
	if err := ResolveInstallPaths(&config.Config{}); err == nil {
		t.Error("ResolveInstallPaths should fail when neither $HOME nor $ANSIBLE_HOME is known")
	}
}

func TestInstallPathRolesFlagBeatsEnv(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("ANSIBLE_ROLES_PATH", "/env/roles")
	cfg := resolvedPaths(t, "/flag/roles", "")
	if cfg.RolesPath != "/flag/roles" {
		t.Errorf("RolesPath = %q, want /flag/roles", cfg.RolesPath)
	}
}

func TestInstallPathRolesFlagListFirstEntry(t *testing.T) {
	clearPathEnv(t)
	cfg := resolvedPaths(t, "/flag/roles:/other", "")
	if cfg.RolesPath != "/flag/roles" {
		t.Errorf("RolesPath = %q, want first entry /flag/roles", cfg.RolesPath)
	}
}

func TestInstallPathRolesEnvFirstEntry(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("ANSIBLE_ROLES_PATH", "/env/roles:/other/roles")
	t.Setenv("ANSIBLE_HOME", "/ignored")
	cfg := resolvedPaths(t, "", "")
	if cfg.RolesPath != "/env/roles" {
		t.Errorf("RolesPath = %q, want first entry /env/roles", cfg.RolesPath)
	}
}

func TestInstallPathRolesEnvSkipsEmptyEntries(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("ANSIBLE_ROLES_PATH", "::/real/roles")
	cfg := resolvedPaths(t, "", "")
	if cfg.RolesPath != "/real/roles" {
		t.Errorf("RolesPath = %q, want /real/roles", cfg.RolesPath)
	}
}

func TestInstallPathRolesAnsibleHome(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("ANSIBLE_HOME", "/alt/home")
	cfg := resolvedPaths(t, "", "")
	if cfg.RolesPath != "/alt/home/roles" {
		t.Errorf("RolesPath = %q, want /alt/home/roles", cfg.RolesPath)
	}
}

func TestInstallPathRolesFlagTilde(t *testing.T) {
	clearPathEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := resolvedPaths(t, "~/my/roles", "")
	if want := filepath.Join(home, "my", "roles"); cfg.RolesPath != want {
		t.Errorf("RolesPath = %q, want %q", cfg.RolesPath, want)
	}
}

func TestInstallPathCollectionsEnvBeatsLegacy(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("ANSIBLE_COLLECTIONS_PATH", "/env/collections")
	t.Setenv("ANSIBLE_COLLECTIONS_PATHS", "/legacy/collections")
	cfg := resolvedPaths(t, "", "")
	if cfg.CollectionsPath != "/env/collections" {
		t.Errorf("CollectionsPath = %q, want /env/collections", cfg.CollectionsPath)
	}
}

func TestInstallPathCollectionsLegacyEnvFirstEntry(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("ANSIBLE_COLLECTIONS_PATHS", "/legacy/collections:/other")
	cfg := resolvedPaths(t, "", "")
	if cfg.CollectionsPath != "/legacy/collections" {
		t.Errorf("CollectionsPath = %q, want first entry /legacy/collections", cfg.CollectionsPath)
	}
}

func TestInstallPathCollectionsAnsibleHome(t *testing.T) {
	clearPathEnv(t)
	t.Setenv("ANSIBLE_HOME", "/alt/home")
	cfg := resolvedPaths(t, "", "")
	if cfg.CollectionsPath != "/alt/home/collections" {
		t.Errorf("CollectionsPath = %q, want /alt/home/collections", cfg.CollectionsPath)
	}
}

func TestInstallPathCollectionsFlagTilde(t *testing.T) {
	clearPathEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := resolvedPaths(t, "", "~/my/collections")
	if want := filepath.Join(home, "my", "collections"); cfg.CollectionsPath != want {
		t.Errorf("CollectionsPath = %q, want %q", cfg.CollectionsPath, want)
	}
}

func TestFirstEntry(t *testing.T) {
	for _, tt := range []struct {
		list string
		want string
		ok   bool
	}{
		{"/roles:/other", "/roles", true},
		{"::/real", "/real", true},
		{"/solo", "/solo", true},
		{"", "", false},
		{"::", "", false},
	} {
		got, ok := firstEntry(tt.list)
		if got != tt.want || ok != tt.ok {
			t.Errorf("firstEntry(%q) = %q, %v, want %q, %v", tt.list, got, ok, tt.want, tt.ok)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/roles", filepath.Join(home, "roles")},
		{"~user/roles", "~user/roles"},
		{"/abs/roles", "/abs/roles"},
		{"rel/roles", "rel/roles"},
	} {
		got, err := expandHome(tt.in)
		if err != nil {
			t.Fatalf("expandHome(%q) error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
