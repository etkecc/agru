package models

import (
	"sort"
	"strings"
)

// UpdatedItem is a struct to hold updated item information
type UpdatedItem struct {
	Role       string
	OldVersion string
	NewVersion string
}

// UpdatedItems is a slice of UpdatedItem
type UpdatedItems []*UpdatedItem

// Add adds a new UpdatedItem to UpdatedItems
func (u UpdatedItems) Add(role, oldVersion, newVersion string) UpdatedItems {
	return append(u, &UpdatedItem{Role: role, OldVersion: oldVersion, NewVersion: newVersion})
}

// String returns a string representation of UpdatedItems
func (u UpdatedItems) String(prefix string) string {
	sort.Slice(u, func(i, j int) bool {
		return u[i].Role < u[j].Role
	})

	var msg strings.Builder
	for _, item := range u {
		if item.OldVersion == "" {
			msg.WriteString("added ")
			msg.WriteString(item.Role)
			msg.WriteString(" (")
			msg.WriteString(item.NewVersion)
			msg.WriteString("); ")
			continue
		}
		msg.WriteString("updated ")
		msg.WriteString(item.Role)
		msg.WriteString(" (")
		msg.WriteString(item.OldVersion)
		msg.WriteString(" -> ")
		msg.WriteString(item.NewVersion)
		msg.WriteString("); ")
	}
	return prefix + msg.String()
}
