package models

import "sort"

// File structure represents requirements.yml file
type File []*Entry

// Sort entries by name
func (r File) Sort() {
	sort.Slice(r, func(i, j int) bool {
		return r[i].GetName() < r[j].GetName()
	})
}

// FileMap structure represents requirements.yml file with roles key
type FileMap struct {
	Roles []*Entry `yaml:"roles"`
}

// Slice returns the slice of roles from of RequirementsFileMap
func (rfm *FileMap) Slice() File {
	return rfm.Roles
}
