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

// Roles returns a new File containing only the entries that are actual roles (i.e., those without an include directive)
func (r File) Roles() File {
	roles := File{}
	for _, entry := range r {
		if entry.Include == "" { // only roles without include directive
			roles = append(roles, entry)
		}
	}
	return roles
}

// FileMap structure represents requirements.yml file with roles key
type FileMap struct {
	Roles []*Entry `yaml:"roles"`
}

// Slice returns the slice of roles from of RequirementsFileMap
func (rfm *FileMap) Slice() File {
	return rfm.Roles
}
