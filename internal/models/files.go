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

// Deduplicate removes duplicate entries by name, keeping the first occurrence
func (r File) Deduplicate() File {
	seen := make(map[string]bool)
	result := File{}
	for _, entry := range r {
		name := entry.GetName()
		if !seen[name] {
			seen[name] = true
			result = append(result, entry)
		}
	}
	return result
}

// RolesLen returns the number of roles without include directive
func (r File) RolesLen() int {
	var size int
	for _, entry := range r {
		if entry.Include == "" { // only roles without include directive
			size++
		}
	}
	return size
}

// FileMap structure represents requirements.yml file with roles key
type FileMap struct {
	Roles []*Entry `yaml:"roles"`
}

// Slice returns the slice of roles from of RequirementsFileMap
func (rfm *FileMap) Slice() File {
	return rfm.Roles
}
