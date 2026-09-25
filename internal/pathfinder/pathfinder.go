// Package pathfinder resolves where agru installs roles and collections.
package pathfinder

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/etkecc/agru/internal/config"
)

// ResolveInstallPaths resolves the roles and collections install paths in place.
func ResolveInstallPaths(cfg *config.Config) error {
	rolesPath, err := resolvePath(cfg.RolesPath, "roles", "ANSIBLE_ROLES_PATH")
	if err != nil {
		return fmt.Errorf("resolving roles path: %w", err)
	}
	cfg.RolesPath = rolesPath
	collectionsPath, err := resolvePath(cfg.CollectionsPath, "collections",
		"ANSIBLE_COLLECTIONS_PATH", "ANSIBLE_COLLECTIONS_PATHS")
	if err != nil {
		return fmt.Errorf("resolving collections path: %w", err)
	}
	cfg.CollectionsPath = collectionsPath
	return nil
}

// resolvePath takes the flag, else the first entry of the first set env var, else $ANSIBLE_HOME/<subdir>.
func resolvePath(flagVal, subdir string, envVars ...string) (string, error) {
	if entry, ok := firstEntry(flagVal); ok {
		return expandHome(entry)
	}
	for _, envVar := range envVars {
		if entry, ok := firstEntry(os.Getenv(envVar)); ok {
			return expandHome(entry)
		}
	}
	home, err := ansibleHome()
	if err != nil {
		return "", err
	}
	return path.Join(home, subdir), nil
}

// firstEntry returns the first non-empty entry of a colon-separated path list.
func firstEntry(list string) (string, bool) {
	for entry := range strings.SplitSeq(list, ":") {
		if entry != "" {
			return entry, true
		}
	}
	return "", false
}

// ansibleHome returns $ANSIBLE_HOME (ansible's own default root), or ~/.ansible.
func ansibleHome() (string, error) {
	if home := os.Getenv("ANSIBLE_HOME"); home != "" {
		return expandHome(home)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return path.Join(home, ".ansible"), nil
}

// expandHome expands a leading ~ to the user's home directory; any other path passes through.
func expandHome(p string) (string, error) {
	rest, ok := strings.CutPrefix(p, "~")
	if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return path.Join(home, rest), nil
}
