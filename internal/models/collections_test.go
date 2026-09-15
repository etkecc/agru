package models

import (
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"
)

func TestCollectionGitURLDerivesNSAndColl(t *testing.T) {
	c := &Collection{Name: "git+https://github.com/ansible-collections/community.docker"}
	c.initSupported()
	if c.ns != "community" || c.coll != "docker" || c.fqcn != "community.docker" {
		t.Errorf("initSupported() ns=%q coll=%q fqcn=%q, want community/docker/community.docker", c.ns, c.coll, c.fqcn)
	}
	if c.Unsupported() != "" {
		t.Errorf("initSupported() unsupported=%q, want empty", c.Unsupported())
	}
}

func TestCollectionBareScalarGitURLSupported(t *testing.T) {
	content := "- git+https://github.com/ansible-collections/community.docker\n"
	var colls Collections
	if err := yaml.Unmarshal([]byte(content), &colls); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(colls) != 1 {
		t.Fatalf("got %d collections, want 1", len(colls))
	}
	if colls[0].Unsupported() != "" {
		t.Errorf("unsupported=%q, want empty for git URL scalar", colls[0].Unsupported())
	}
	if colls[0].GetFQCN() != "community.docker" {
		t.Errorf("GetFQCN()=%q, want community.docker", colls[0].GetFQCN())
	}
}

func TestCollectionBareGalaxyNameUnsupported(t *testing.T) {
	content := "- community.general\n"
	var colls Collections
	if err := yaml.Unmarshal([]byte(content), &colls); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(colls) != 1 {
		t.Fatalf("got %d collections, want 1", len(colls))
	}
	if colls[0].Unsupported() != ReasonGalaxySource {
		t.Errorf("unsupported=%q, want %q", colls[0].Unsupported(), ReasonGalaxySource)
	}

	// Round-trip: should emit verbatim as scalar
	out, err := yaml.Marshal(colls)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "- community.general" {
		t.Errorf("Marshal = %q, want %q", got, "- community.general")
	}
}

func TestCollectionUnsupportedReasons(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		reason string
	}{
		{
			name:   "extra keys (source)",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  source: https://galaxy.ansible.com\n  version: 1.0.0\n",
			reason: ReasonExtraKeys,
		},
		{
			name:   "extra keys (signatures)",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  signatures: []\n",
			reason: ReasonExtraKeys,
		},
		{
			name:   "galaxy source bare name in mapping",
			yaml:   "- name: community.general\n  version: 8.0.0\n",
			reason: ReasonGalaxySource,
		},
		{
			name:   "subdirectory fragment",
			yaml:   "- name: git+https://github.com/org/ns.coll#/subdir\n",
			reason: ReasonSubdirFragment,
		},
		{
			name:   "unsupported type",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  type: url\n",
			reason: ReasonUnsupportedType("url"),
		},
		{
			name:   "version range (>=)",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  version: \">=3.0.0\"\n",
			reason: ReasonVersionRange,
		},
		{
			name:   "version range (comma)",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  version: \">=1.0.0,<2.0.0\"\n",
			reason: ReasonVersionRange,
		},
		{
			name:   "version range (!=)",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  version: \"!=3.0.0\"\n",
			reason: ReasonVersionRange,
		},
		{
			name:   "version range (~=)",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  version: \"~=3.0.0\"\n",
			reason: ReasonVersionRange,
		},
		{
			name:   "version range (*)",
			yaml:   "- name: git+https://github.com/org/ns.coll\n  version: \"*\"\n",
			reason: ReasonVersionRange,
		},
		{
			name:   "bad basename (no dot)",
			yaml:   "- name: git+https://github.com/org/badname\n  version: 1.0.0\n",
			reason: ReasonBadBasename,
		},
		{
			name:   "bad basename (empty ns)",
			yaml:   "- name: git+https://github.com/org/.coll\n  version: 1.0.0\n",
			reason: ReasonBadBasename,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var colls Collections
			if err := yaml.Unmarshal([]byte(tt.yaml), &colls); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if len(colls) != 1 {
				t.Fatalf("got %d collections, want 1", len(colls))
			}
			if colls[0].Unsupported() != tt.reason {
				t.Errorf("unsupported=%q, want %q", colls[0].Unsupported(), tt.reason)
			}
		})
	}
}

func TestCollectionHostileBasename(t *testing.T) {
	// ns..git would normalize upward on join; should be caught as unsupported
	content := "- name: git+https://x/ns..git\n  version: 1.0.0\n"
	var colls Collections
	if err := yaml.Unmarshal([]byte(content), &colls); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(colls) != 1 {
		t.Fatalf("got %d collections, want 1", len(colls))
	}
	if colls[0].Unsupported() != ReasonBadBasename {
		t.Errorf("unsupported=%q, want %q (traversal guard)", colls[0].Unsupported(), ReasonBadBasename)
	}
}

func TestCollectionIsInstalled(t *testing.T) {
	c := &Collection{ns: "community", coll: "docker", fqcn: "community.docker", Version: "3.13.0"}

	// Missing manifest -> not installed
	fsys := fstest.MapFS{}
	if c.IsInstalled(fsys) {
		t.Error("IsInstalled() = true, want false for missing manifest")
	}

	// Correct version -> installed
	manifest := `{"collection_info": {"namespace": "community", "name": "docker", "version": "3.13.0"}, "format": "1.0.0"}`
	fsys = fstest.MapFS{
		"ansible_collections/community/docker/MANIFEST.json": &fstest.MapFile{
			Data: []byte(manifest),
		},
	}
	if !c.IsInstalled(fsys) {
		t.Error("IsInstalled() = false, want true for matching version")
	}

	// Wrong version -> not installed
	c.Version = "3.14.0"
	if c.IsInstalled(fsys) {
		t.Error("IsInstalled() = true, want false for version mismatch")
	}

	// Empty version -> always false (HEAD mode)
	c.Version = ""
	if c.IsInstalled(fsys) {
		t.Error("IsInstalled() = true, want false for empty version (HEAD mode)")
	}
}

func TestCollectionGetInstalledVersion(t *testing.T) {
	c := &Collection{ns: "community", coll: "docker", fqcn: "community.docker"}
	manifest := `{"collection_info": {"namespace": "community", "name": "docker", "version": "3.13.0"}, "format": "1.0.0"}`
	fsys := fstest.MapFS{
		"ansible_collections/community/docker/MANIFEST.json": &fstest.MapFile{
			Data: []byte(manifest),
		},
	}
	if v := c.GetInstalledVersion(fsys); v != "3.13.0" {
		t.Errorf("GetInstalledVersion()=%q, want 3.13.0", v)
	}

	// Missing -> empty
	fsys2 := fstest.MapFS{}
	if v := c.GetInstalledVersion(fsys2); v != "" {
		t.Errorf("GetInstalledVersion()=%q, want empty for missing", v)
	}
}

func TestCollectionGenerateManifest(t *testing.T) {
	c := &Collection{ns: "community", coll: "docker", fqcn: "community.docker", Version: "3.13.0"}
	galaxy := map[string]any{
		"namespace": "community",
		"name":      "docker",
		"version":   "3.12.0",
		"authors":   []string{"ansible"},
	}

	data, err := c.GenerateManifest(galaxy)
	if err != nil {
		t.Fatalf("GenerateManifest error: %v", err)
	}

	// Should use c.Version (3.13.0) as the pinned version
	if !strings.Contains(string(data), `"version":"3.13.0"`) && !strings.Contains(string(data), `"version": "3.13.0"`) {
		t.Errorf("GenerateManifest should use c.Version (3.13.0), got: %s", data)
	}

	// Empty version should fall back to galaxy version
	c.Version = ""
	data, err = c.GenerateManifest(galaxy)
	if err != nil {
		t.Fatalf("GenerateManifest error: %v", err)
	}
	if !strings.Contains(string(data), `"version":"3.12.0"`) && !strings.Contains(string(data), `"version": "3.12.0"`) {
		t.Errorf("GenerateManifest should fall back to galaxy version (3.12.0), got: %s", data)
	}
}

func TestCollectionsSort(t *testing.T) {
	colls := Collections{
		&Collection{fqcn: "zebra.alpha"},
		&Collection{fqcn: "alpha.beta"},
		&Collection{fqcn: "mango.zulu"},
	}
	colls.Sort()
	expected := []string{"alpha.beta", "mango.zulu", "zebra.alpha"}
	for i, c := range colls {
		if c.GetFQCN() != expected[i] {
			t.Errorf("Sort()[%d] = %q, want %q", i, c.GetFQCN(), expected[i])
		}
	}
}

func TestCollectionsDeduplicate(t *testing.T) {
	colls := Collections{
		&Collection{fqcn: "community.docker", Version: "3.13.0"},
		&Collection{fqcn: "community.general"},
		&Collection{fqcn: "community.docker", Version: "3.14.0"}, // duplicate
	}
	result := colls.Deduplicate()
	if len(result) != 2 {
		t.Fatalf("Deduplicate() len=%d, want 2", len(result))
	}
	// First occurrence kept
	if result[0].Version != "3.13.0" {
		t.Errorf("Deduplicate() kept version %q, want first occurrence 3.13.0", result[0].Version)
	}
}

func TestCollectionParseManifest(t *testing.T) {
	data := []byte(`{"collection_info": {"namespace": "community", "name": "docker", "version": "3.13.0"}, "format": "1.0.0"}`)
	info, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest error: %v", err)
	}
	if info["version"] != "3.13.0" {
		t.Errorf("version=%q, want 3.13.0", info["version"])
	}

	// Missing collection_info -> error
	_, err = ParseManifest([]byte(`{}`))
	if err == nil {
		t.Error("ParseManifest should error on missing collection_info")
	}
}

func TestCollectionGetPath(t *testing.T) {
	c := &Collection{ns: "community", coll: "docker"}
	p := c.GetPath("/collections")
	want := "/collections/ansible_collections/community/docker"
	if p != want {
		t.Errorf("GetPath()=%q, want %q", p, want)
	}

	// Unsupported entry returns empty
	c2 := &Collection{unsupported: ReasonGalaxySource}
	if p2 := c2.GetPath("/collections"); p2 != "" {
		t.Errorf("GetPath()=%q, want empty for unsupported", p2)
	}
}

func TestCollectionRoundTripUnsupportedVerbatim(t *testing.T) {
	input := `- community.general
- name: git+https://github.com/org/ns.coll
  version: ">=3.0.0"
`
	var colls Collections
	if err := yaml.Unmarshal([]byte(input), &colls); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(colls) != 2 {
		t.Fatalf("got %d collections, want 2", len(colls))
	}

	out, err := yaml.Marshal(colls)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := strings.TrimSpace(string(out))
	// Should preserve the unsupported entries verbatim
	if !strings.Contains(got, "community.general") {
		t.Errorf("Marshal lost community.general: %s", got)
	}
	if !strings.Contains(got, ">=3.0.0") {
		t.Errorf("Marshal lost version range: %s", got)
	}
}

func TestCollectionScalarGitURLRoundTrip(t *testing.T) {
	input := `- git+https://github.com/ansible-collections/community.docker
`
	var colls Collections
	if err := yaml.Unmarshal([]byte(input), &colls); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(colls) != 1 {
		t.Fatalf("got %d collections, want 1", len(colls))
	}
	if colls[0].Unsupported() != "" {
		t.Fatalf("unsupported=%q, want empty", colls[0].Unsupported())
	}

	// Supported scalar becomes a map on round-trip
	out, err := yaml.Marshal(colls)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "name:") {
		t.Errorf("Marshal should emit map form, got: %s", got)
	}
}

func TestEntryUnsupported(t *testing.T) {
	e := &Entry{unsupported: "not a git source"}
	if e.Unsupported() != "not a git source" {
		t.Errorf("Unsupported()=%q, want not a git source", e.Unsupported())
	}
	e2 := &Entry{}
	if e2.Unsupported() != "" {
		t.Errorf("Unsupported()=%q, want empty", e2.Unsupported())
	}
}
