package main

import (
	"log"
	"os"
	"path"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// GalaxyInstallInfo is meta/.galaxy_install_info struct
type GalaxyInstallInfo struct {
	InstallDate string `yaml:"install_date"`
	Version     string `yaml:"version"`
}

// RequirementsEntry is requirements.yml's entry structure
type RequirementsEntry struct {
	Src     string `yaml:"src,omitempty"`
	Version string `yaml:"version,omitempty"`
	Name    string `yaml:"name,omitempty"`
	Include string `yaml:"include,omitempty"`
}

// GetName returns entry name
func (e RequirementsEntry) GetName() string {
	if e.Name != "" {
		return e.Name
	}
	return strings.TrimSuffix(path.Base(e.Src), ".git")
}

// GetPath returns path to the entry in filesystem
func (e RequirementsEntry) GetPath() string {
	return path.Join(rolesPath, e.GetName())
}

// GetInstallInfoPath returns path to .galaxy_install_info for that entry
func (e RequirementsEntry) GetInstallInfoPath() string {
	return path.Join(e.GetPath(), "meta", ".galaxy_install_info")
}

// GetInstallInfo parses .galaxy_install_info and returns parsed info
func (e RequirementsEntry) GetInstallInfo() GalaxyInstallInfo {
	_, err := os.Stat(e.GetInstallInfoPath())
	if err != nil && os.IsNotExist(err) {
		return GalaxyInstallInfo{}
	}

	fileb, err := os.ReadFile(e.GetInstallInfoPath())
	if err != nil {
		log.Println("ERROR: ", err)
		return GalaxyInstallInfo{}
	}

	var info GalaxyInstallInfo
	if err := yaml.Unmarshal(fileb, &info); err != nil {
		log.Println("ERROR: ", err)
	}

	return info
}

// GenerateInstallInfo generates fresh install info from current state of the entry struct
func (e RequirementsEntry) GenerateInstallInfo() ([]byte, error) {
	info := GalaxyInstallInfo{
		InstallDate: time.Now().Format("Mon 02 Jan 2006 03:04:05 PM "), // the trailing space is done by ansible-galaxy
		Version:     e.Version,
	}
	return yaml.Marshal(info)
}

// IsInstalled checks if that entry with that specific version is installed
func (e RequirementsEntry) IsInstalled() bool {
	_, err := os.Stat(e.GetPath())
	if err != nil && os.IsNotExist(err) {
		return false
	}

	if e.Version != e.GetInstallInfo().Version {
		return false
	}
	return true
}
