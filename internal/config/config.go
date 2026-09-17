// Package config holds agru's runtime configuration, shared by the CLI and main's dispatch.
package config

// Config holds the configuration for a single agru run, derived from CLI flags.
type Config struct {
	RequirementsPath string
	RolesPath        string
	CollectionsPath  string
	DeleteName       string
	Limit            int
	ListInstalled    bool
	InstallMissing   bool
	UpdateFile       bool
	Cleanup          bool
	Verbose          bool
	NoTUI            bool // accepted for compatibility; interactive mode was removed
}
