package models

import (
	"strings"
	"testing"
)

func TestUpdatedItemsAdd(t *testing.T) {
	var u UpdatedItems
	u = u.Add("role-a", "v1.0.0", "v2.0.0")
	u = u.Add("role-b", "", "v1.0.0")

	if len(u) != 2 {
		t.Fatalf("Add() len = %d, want 2", len(u))
	}
	if u[0].Role != "role-a" || u[0].OldVersion != "v1.0.0" || u[0].NewVersion != "v2.0.0" {
		t.Errorf("Add()[0] = %+v, unexpected values", u[0])
	}
	if u[1].Role != "role-b" || u[1].OldVersion != "" || u[1].NewVersion != "v1.0.0" {
		t.Errorf("Add()[1] = %+v, unexpected values", u[1])
	}
}

func TestUpdatedItemsStringUpdated(t *testing.T) {
	var u UpdatedItems
	u = u.Add("role-a", "v1.0.0", "v2.0.0")
	result := u.String("changes:\n")

	if !strings.Contains(result, "changes:\n") {
		t.Errorf("String() missing prefix, got: %q", result)
	}
	if !strings.Contains(result, "updated role-a") {
		t.Errorf("String() missing 'updated role-a', got: %q", result)
	}
	if !strings.Contains(result, "v1.0.0 -> v2.0.0") {
		t.Errorf("String() missing version transition, got: %q", result)
	}
}

func TestUpdatedItemsStringAdded(t *testing.T) {
	var u UpdatedItems
	u = u.Add("role-b", "", "v1.0.0")
	result := u.String("")

	if !strings.Contains(result, "added role-b") {
		t.Errorf("String() missing 'added role-b', got: %q", result)
	}
	if !strings.Contains(result, "v1.0.0") {
		t.Errorf("String() missing version, got: %q", result)
	}
}

func TestUpdatedItemsStringSorted(t *testing.T) {
	var u UpdatedItems
	u = u.Add("zebra", "v1.0.0", "v2.0.0")
	u = u.Add("alpha", "v1.0.0", "v2.0.0")
	u = u.Add("mango", "v1.0.0", "v2.0.0")

	result := u.String("")
	alphaIdx := strings.Index(result, "alpha")
	mangoIdx := strings.Index(result, "mango")
	zebraIdx := strings.Index(result, "zebra")

	if alphaIdx > mangoIdx || mangoIdx > zebraIdx {
		t.Errorf("String() output not sorted alphabetically: %q", result)
	}
}
