//go:build e2e

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// commitRe is the shape consumers' tooling expects in install_commit.
var commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// installInfo mirrors meta/.galaxy_install_info, the file consumers commit and read back.
type installInfo struct {
	InstallCommit string `yaml:"install_commit"`
	Version       string `yaml:"version"`
}

// requireInstalled asserts the role landed in the roles path at the pinned version.
func (p *project) requireInstalled(role, wantVersion string) {
	p.t.Helper()
	rel := "roles/galaxy/" + role
	if _, err := os.Stat(p.path(rel + "/meta")); err != nil {
		p.t.Errorf("%s: %s was not installed (no meta dir) after agru %s\n%s",
			p.caller.label(), rel, strings.Join(p.last.args, " "), p.last.output())
		return
	}
	info := p.installInfo(role)
	if info.Version != wantVersion {
		p.t.Errorf("%s: %s install info version = %q, want %q (the pinned version)", p.caller.label(), rel, info.Version, wantVersion)
	}
	if !commitRe.MatchString(info.InstallCommit) {
		p.t.Errorf("%s: %s install_commit = %q, want a 40 char commit: reinstall checks and etkecc's scripts read it",
			p.caller.label(), rel, info.InstallCommit)
	}
	if _, err := os.Stat(p.path(rel + "/.git")); err == nil {
		p.t.Errorf("%s: %s carries a .git dir: agru installs archives, and etkecc commits these trees", p.caller.label(), rel)
	}
}

// installInfo parses a role's meta/.galaxy_install_info.
func (p *project) installInfo(role string) installInfo {
	p.t.Helper()
	rel := "roles/galaxy/" + role + "/meta/.galaxy_install_info"
	var info installInfo
	if err := yaml.Unmarshal([]byte(p.read(rel)), &info); err != nil {
		p.t.Fatalf("%s: parsing %s: %v", p.caller.label(), rel, err)
	}
	return info
}

// requireTextEqual compares whole files: churn, dropped extras and reordering all fail loudly.
func (p *project) requireTextEqual(rel, got, want string) {
	p.t.Helper()
	if got == want {
		return
	}
	p.t.Errorf("%s: %s is not what the run should have left behind:\n%s", p.caller.label(), rel, firstDiff(got, want))
}

// replacePin is the only edit -u is allowed to make: one role's version line, nothing else.
func replacePin(contents, oldVersion, newVersion string) string {
	return strings.Replace(contents, "\n  version: "+oldVersion+"\n", "\n  version: "+newVersion+"\n", 1)
}

// firstDiff renders the first differing line pair so a red run says what actually changed.
func firstDiff(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(gotLines), len(wantLines)) {
		if lineAt(gotLines, i) != lineAt(wantLines, i) {
			return fmt.Sprintf("  line %d: want %q, got %q (%d lines vs %d)",
				i+1, lineAt(wantLines, i), lineAt(gotLines, i), len(wantLines), len(gotLines))
		}
	}
	return fmt.Sprintf("  same lines, different bytes (trailing newline?): %d lines vs %d", len(wantLines), len(gotLines))
}

// lineAt returns the line at i, or a marker when that file is shorter.
func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<missing>"
}

// treeHashes hashes every file under rel, so a no-op re-run can be proven byte for byte.
func (p *project) treeHashes(rel string) map[string]string {
	p.t.Helper()
	root := p.path(rel)
	hashes := map[string]string{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(content)
		name, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		hashes[name] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		p.t.Fatalf("%s: hashing %s: %v", p.caller.label(), rel, err)
	}
	return hashes
}

// requireSameTree fails when a re-run rewrote or dropped files it should have left alone.
func (p *project) requireSameTree(rel string, before, after map[string]string) {
	p.t.Helper()
	changed := make([]string, 0, len(after))
	for name, sum := range after {
		if before[name] != sum {
			changed = append(changed, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			changed = append(changed, name+" (gone)")
		}
	}
	if len(changed) == 0 {
		return
	}
	sort.Strings(changed)
	p.t.Errorf("%s: %s changed under a run that should have been a no-op: %s", p.caller.label(), rel, strings.Join(changed, ", "))
}

// requireNoInstalls fails when a re-run reported roles that were already installed at their pin.
func (p *project) requireNoInstalls(r runResult, roles ...string) {
	p.t.Helper()
	for _, role := range roles {
		if strings.Contains(r.output(), role) {
			p.t.Errorf("%s: re-run mentions %s: it treated an installed pin as missing\n%s", p.caller.label(), role, r.output())
		}
	}
}

// newestTag mirrors agru's own check: newest tag by version:refname sort, peeled refs unwrapped.
func newestTag(t *testing.T, src string) string {
	t.Helper()
	repo := strings.Replace(src, "git+", "", 1)
	cmd := exec.Command("git", "ls-remote", "-tq", "--sort=-version:refname", repo)
	cmd.Env = append(scratchEnv(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-remote %s: %v", repo, err)
	}
	for entry := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if _, tag, ok := strings.Cut(entry, "refs/tags/"); ok {
			return strings.TrimSuffix(tag, "^{}")
		}
	}
	t.Fatalf("no tags on %s: this case cannot exercise -u against it", repo)
	return ""
}
