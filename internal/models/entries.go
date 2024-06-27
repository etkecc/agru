package models

import (
	"os"
	"path"
	"strings"
	"time"

	"gitlab.com/etke.cc/int/agru/internal/utils"
	"gopkg.in/yaml.v3"
)

// GalaxyInstallInfo is meta/.galaxy_install_info struct
type GalaxyInstallInfo struct {
	InstallDate string `yaml:"install_date"`
	Version     string `yaml:"version"`
}

// Entry is requirements.yml's entry structure
type Entry struct {
	name             string  `yaml:"-"`
	Src              string  `yaml:"src,omitempty"`
	Version          string  `yaml:"version,omitempty"`
	Name             string  `yaml:"name,omitempty"`
	Include          string  `yaml:"include,omitempty"`
	ActivationPrefix *string `yaml:"activation_prefix,omitempty"`
}

// GetName returns entry name with the following priority order
// 1. name from the requirements.yml file (if set)
// 2. name, generated from the entry's src
func (e *Entry) GetName() string {
	if e.name != "" {
		return e.name
	}

	if e.Name != "" {
		e.name = e.Name
		return e.name
	}

	e.name = strings.TrimSuffix(path.Base(e.Src), ".git")
	return e.name
}

// GetPath returns path to the entry in filesystem
func (e *Entry) GetPath(rolesPath string) string {
	return path.Join(rolesPath, e.GetName())
}

// GetInstallInfoPath returns path to .galaxy_install_info for that entry
func (e *Entry) GetInstallInfoPath(rolesPath string) string {
	return path.Join(e.GetPath(rolesPath), "meta", ".galaxy_install_info")
}

// GetInstallInfo parses .galaxy_install_info and returns parsed info
func (e *Entry) GetInstallInfo(rolesPath string) GalaxyInstallInfo {
	_, err := os.Stat(e.GetInstallInfoPath(rolesPath))
	if err != nil && os.IsNotExist(err) {
		return GalaxyInstallInfo{}
	}

	fileb, err := os.ReadFile(e.GetInstallInfoPath(rolesPath))
	if err != nil {
		utils.Log("ERROR:", err)
		return GalaxyInstallInfo{}
	}

	var info GalaxyInstallInfo
	if err := yaml.Unmarshal(fileb, &info); err != nil {
		utils.Log("ERROR:", err)
	}

	return info
}

// GenerateInstallInfo generates fresh install info from current state of the entry struct
func (e *Entry) GenerateInstallInfo() ([]byte, error) {
	info := GalaxyInstallInfo{
		InstallDate: time.Now().Format("Mon 02 Jan 2006 03:04:05 PM "), // the trailing space is done by ansible-galaxy
		Version:     e.Version,
	}
	return yaml.Marshal(info)
}

// IsInstalled checks if that entry with that specific version is installed
func (e *Entry) IsInstalled(rolesPath string) bool {
	_, err := os.Stat(e.GetPath(rolesPath))
	if err != nil && os.IsNotExist(err) {
		return false
	}

	if e.Version != e.GetInstallInfo(rolesPath).Version {
		return false
	}
	return true
}
