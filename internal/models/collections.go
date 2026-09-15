package models

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/goccy/go-json"
	"gopkg.in/yaml.v3"
)

// Unsupported-reason constants - exact, rendered everywhere.
const (
	ReasonGalaxySource   = "galaxy source, git only"
	ReasonCustomSource   = "custom source, git only"
	ReasonVersionRange   = "version range, exact pin required"
	ReasonSubdirFragment = "subdirectory fragment unsupported"
	ReasonExtraKeys      = "unsupported extra keys"
	ReasonBadBasename    = "repo name is not namespace.collection"
)

// ReasonUnsupportedType formats the unsupported-type reason.
func ReasonUnsupportedType(t string) string { return "unsupported type: " + t + ", git only" }

// Collections is a sorted, deduplicated slice of Collection pointers.
type Collections []*Collection

// Collection represents a single collection entry from requirements.yml.
type Collection struct {
	Name    string `yaml:"name"`              // git URL for supported entries
	Version string `yaml:"version,omitempty"` // exact version or empty
	Type    string `yaml:"type,omitempty"`    // re-emitted only if present in source

	fqcn        string     // cached ns.coll
	ns          string     // namespace
	coll        string     // collection name
	raw         *yaml.Node // verbatim re-emit for unsupported entries
	unsupported string     // reason, empty = supported
}

// UnmarshalYAML implements yaml.Unmarshaler for the collections sequence.
func (c *Collections) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("collections must be a sequence")
	}
	*c = make(Collections, 0, len(value.Content))
	for _, item := range value.Content {
		col := &Collection{}
		switch item.Kind {
		case yaml.ScalarNode:
			col.Name = item.Value
			if IsGitURL(item.Value) {
				col.initSupported()
			} else {
				col.unsupported = ReasonGalaxySource
				col.raw = item
			}
		case yaml.MappingNode:
			col.parseMapping(item)
		default:
			return fmt.Errorf("unsupported YAML node kind %d in collections sequence", item.Kind)
		}
		*c = append(*c, col)
	}
	return nil
}

// MarshalYAML implements yaml.Marshaler for the collections sequence.
func (c Collections) MarshalYAML() (any, error) { //nolint:unparam // yaml.Marshaler interface
	out := make([]any, 0, len(c))
	for _, col := range c {
		out = append(out, col)
	}
	return out, nil
}

// MarshalYAML implements yaml.Marshaler for a single collection entry.
func (c *Collection) MarshalYAML() (any, error) { //nolint:unparam // yaml.Marshaler interface
	if c.unsupported != "" {
		return c.raw, nil // verbatim re-emit
	}
	m := map[string]any{
		"name": c.Name,
	}
	if c.Version != "" {
		m["version"] = c.Version
	}
	if c.Type != "" {
		m["type"] = c.Type
	}
	return m, nil
}

// parseMapping decodes a YAML mapping node into the collection fields.
func (c *Collection) parseMapping(node *yaml.Node) {
	var name, version, typeStr string
	extraKeys := false
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]
		switch key {
		case "name":
			name = val.Value
		case "version":
			version = val.Value
		case "type":
			typeStr = val.Value
		default:
			extraKeys = true
		}
	}

	c.Name = name
	c.Version = version
	c.Type = typeStr

	if extraKeys {
		c.unsupported = ReasonExtraKeys
		c.raw = node
		return
	}
	if !IsGitURL(name) {
		c.unsupported = ReasonGalaxySource
		c.raw = node
		return
	}
	if strings.Contains(name, "#") {
		c.unsupported = ReasonSubdirFragment
		c.raw = node
		return
	}
	if typeStr != "" && typeStr != "git" {
		c.unsupported = ReasonUnsupportedType(typeStr)
		c.raw = node
		return
	}
	if version != "" && isRangeVersion(version) {
		c.unsupported = ReasonVersionRange
		c.raw = node
		return
	}

	c.initSupported()
}

// initSupported derives ns, coll, fqcn from the git URL basename.
func (c *Collection) initSupported() {
	repoURL := strings.TrimPrefix(c.Name, "git+")
	base := path.Base(repoURL)
	base = strings.TrimSuffix(base, ".git")

	parts := strings.SplitN(base, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		c.unsupported = ReasonBadBasename
		return
	}
	if !isValidIdentifier(parts[0]) || !isValidIdentifier(parts[1]) {
		c.unsupported = ReasonBadBasename
		return
	}
	c.ns = parts[0]
	c.coll = parts[1]
	c.fqcn = parts[0] + "." + parts[1]
}

// GetFQCN returns the fully qualified collection name.
func (c *Collection) GetFQCN() string {
	if c.fqcn != "" {
		return c.fqcn
	}
	return c.Name
}

// Unsupported returns the reason this entry is unsupported, empty when supported.
func (c *Collection) Unsupported() string {
	return c.unsupported
}

// Namespace returns the collection namespace.
func (c *Collection) Namespace() string { return c.ns }

// CollectionName returns the collection name (without namespace).
func (c *Collection) CollectionName() string { return c.coll }

// GetRepoURL returns the git URL without the git+ prefix.
func (c *Collection) GetRepoURL() string {
	return strings.TrimPrefix(c.Name, "git+")
}

// GetPath returns the filesystem path under collectionsPath.
func (c *Collection) GetPath(collectionsPath string) string {
	if c.ns == "" || c.coll == "" {
		return ""
	}
	return path.Join(collectionsPath, "ansible_collections", c.ns, c.coll)
}

// IsInstalled checks if the collection is installed at the given version.
func (c *Collection) IsInstalled(fsys fs.FS) bool {
	if c.Version == "" {
		return false // HEAD mode, forced reinstall
	}
	rel := path.Join("ansible_collections", c.ns, c.coll, "MANIFEST.json")
	data, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return false
	}
	info, err := ParseManifest(data)
	if err != nil {
		return false
	}
	ver, ok := info["version"].(string)
	if !ok {
		return false
	}
	return ver == c.Version
}

// GetInstalledVersion returns the installed version from MANIFEST.json.
func (c *Collection) GetInstalledVersion(fsys fs.FS) string {
	rel := path.Join("ansible_collections", c.ns, c.coll, "MANIFEST.json")
	data, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return ""
	}
	info, err := ParseManifest(data)
	if err != nil {
		return ""
	}
	ver, _ := info["version"].(string) //nolint:errcheck // type assertion discarding ok is safe; empty string on mismatch
	return ver
}

// GenerateManifest creates MANIFEST.json bytes from a galaxy.yml map, replacing version with the pinned one.
func (c *Collection) GenerateManifest(galaxy map[string]any) ([]byte, error) {
	version := c.Version
	if version == "" {
		if v, ok := galaxy["version"].(string); ok && v != "" {
			version = v
		}
	}

	info := map[string]any{
		"namespace": c.ns,
		"name":      c.coll,
		"version":   version,
	}
	copyGalaxyFields(galaxy, info)

	manifest := map[string]any{
		"collection_info": info,
		"format":          "1.0.0",
	}
	return json.Marshal(manifest)
}

// copyGalaxyFields copies galaxy metadata into info, excluding version/namespace/name.
func copyGalaxyFields(galaxy, info map[string]any) {
	if ci, ok := galaxy["collection_info"]; ok {
		if m, ok := ci.(map[string]any); ok {
			for k, v := range m {
				if k != "version" {
					info[k] = v
				}
			}
		}
		return
	}
	for k, v := range galaxy {
		if k != "namespace" && k != "name" && k != "version" {
			info[k] = v
		}
	}
}

// ParseManifest parses a MANIFEST.json and returns the collection_info map.
func ParseManifest(data []byte) (map[string]any, error) {
	var doc struct {
		Info map[string]any `json:"collection_info"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Info == nil {
		return nil, fmt.Errorf("MANIFEST.json missing collection_info")
	}
	return doc.Info, nil
}

// Sort sorts collections by FQCN.
func (c Collections) Sort() {
	sort.Slice(c, func(i, j int) bool {
		return c[i].GetFQCN() < c[j].GetFQCN()
	})
}

// Deduplicate removes duplicate entries by FQCN, keeping the first occurrence.
func (c Collections) Deduplicate() Collections {
	seen := make(map[string]bool)
	result := make(Collections, 0, len(c))
	for _, col := range c {
		fqcn := col.GetFQCN()
		if !seen[fqcn] {
			seen[fqcn] = true
			result = append(result, col)
		}
	}
	return result
}

// IsGitURL checks if a string looks like a git URL (https, http, git+, git@, ssh).
func IsGitURL(s string) bool {
	return strings.HasPrefix(s, "git+") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "ssh://")
}

// isRangeVersion checks if a version string is a range (unsupported) vs an exact pin.
func isRangeVersion(v string) bool {
	if v == "" {
		return false
	}
	if strings.HasPrefix(v, "!=") {
		return true
	}
	return strings.ContainsAny(v, "><,*!|~")
}

// isValidIdentifier checks that a string matches [a-zA-Z0-9_]+.
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
