package models

import (
	"testing"
)

func TestFileSort(t *testing.T) {
	f := File{
		{Name: "zebra"},
		{Name: "alpha"},
		{Name: "mango"},
	}
	f.Sort()
	expected := []string{"alpha", "mango", "zebra"}
	for i, e := range f {
		if e.GetName() != expected[i] {
			t.Errorf("Sort()[%d] = %q, want %q", i, e.GetName(), expected[i])
		}
	}
}

func TestFileDeduplicate(t *testing.T) {
	f := File{
		{Name: "role-a"},
		{Name: "role-b"},
		{Name: "role-a"}, // duplicate
		{Name: "role-c"},
		{Name: "role-b"}, // duplicate
	}
	result := f.Deduplicate()
	if len(result) != 3 {
		t.Errorf("Deduplicate() len = %d, want 3", len(result))
	}
	names := map[string]bool{}
	for _, e := range result {
		names[e.GetName()] = true
	}
	for _, expected := range []string{"role-a", "role-b", "role-c"} {
		if !names[expected] {
			t.Errorf("Deduplicate() missing %q", expected)
		}
	}
}

func TestFileDeduplicateKeepsFirst(t *testing.T) {
	f := File{
		{Name: "role-a", Version: "v1.0.0"},
		{Name: "role-a", Version: "v2.0.0"}, // duplicate, should be dropped
	}
	result := f.Deduplicate()
	if len(result) != 1 {
		t.Fatalf("Deduplicate() len = %d, want 1", len(result))
	}
	if result[0].Version != "v1.0.0" {
		t.Errorf("Deduplicate() kept version %q, want first occurrence v1.0.0", result[0].Version)
	}
}

func TestFileRolesLen(t *testing.T) {
	f := File{
		{Name: "role-a"},
		{Include: "other-requirements.yml"}, // not a role
		{Name: "role-b"},
		{Include: "more-requirements.yml"}, // not a role
		{Name: "role-c"},
	}
	got := f.RolesLen()
	if got != 3 {
		t.Errorf("RolesLen() = %d, want 3", got)
	}
}

func TestFileMapSlice(t *testing.T) {
	entries := []*Entry{
		{Name: "role-a"},
		{Name: "role-b"},
	}
	fm := FileMap{Roles: entries}
	result := fm.Slice()
	if len(result) != 2 {
		t.Errorf("Slice() len = %d, want 2", len(result))
	}
	for i, e := range result {
		if e.GetName() != entries[i].GetName() {
			t.Errorf("Slice()[%d] = %q, want %q", i, e.GetName(), entries[i].GetName())
		}
	}
}
