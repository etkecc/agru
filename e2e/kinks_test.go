//go:build e2e

package e2e

import "testing"

// pinMovePins pairs a tag pin one release behind with a sha pin, so a move can be told from immunity.
const pinMovePins = `---

- src: git+https://github.com/mother-of-all-self-hosting/ansible-role-cleanup.git
  version: v1.0.0-1
  name: cleanup
- src: git+https://github.com/devture/com.devture.ansible.role.playbook_state_preserver.git
  version: dd6e15246b7a9a2d921e0b3f9cd8a4a917a1bb2f
  name: playbook_state_preserver
`

// kink pins down a shape rather than a recipe: the pin kinds the three repos cannot stop using.
var kink = &caller{
	repo:   "consumers",
	recipe: "shape: mixed pins",
	source: "etkecc pins swap and ufw to main, mdad and mash pin four tagless roles by sha",
	argv:   []string{"-p", "roles/galaxy/"},
}

// TestInstallIsIdempotent covers `just roles` twice: a second run must install and rewrite nothing.
func TestInstallIsIdempotent(t *testing.T) {
	p := newProject(t, kink, map[string]string{"requirements.yml": mdadPinned})
	p.requireOK(p.run())
	before := p.treeHashes("roles/galaxy")
	second := p.run()
	p.requireOK(second)
	p.requireNoInstalls(second, "auxiliary", "docker", "playbook_state_preserver")
	p.requireSameTree("roles/galaxy", before, p.treeHashes("roles/galaxy"))
	p.requireInstalled("docker", mdadDockerPin)
}

// TestReinstallOnPinMove covers etkecc's committed roles dir: a moved pin must move the role.
func TestReinstallOnPinMove(t *testing.T) {
	p := newProject(t, kink, map[string]string{"requirements.yml": pinMovePins})
	p.requireOK(p.run())
	p.requireInstalled("cleanup", etkeccCleanupPin)
	bumped := newestTag(t, etkeccCleanupSrc)
	if bumped == etkeccCleanupPin {
		t.Fatalf("fixture is no longer behind: pin %s at an older tag of %s", etkeccCleanupPin, etkeccCleanupSrc)
	}
	p.write("requirements.yml", replacePin(pinMovePins, etkeccCleanupPin, bumped))
	p.requireOK(p.run())
	p.requireInstalled("cleanup", bumped)
	p.requireInstalled("playbook_state_preserver", mdadPreserverSHA)
}

// TestUpdateIsStableAcrossRuns: the hourly workflow commits on a diff, so a second -u must change nothing.
func TestUpdateIsStableAcrossRuns(t *testing.T) {
	p := newProject(t, etkeccCI, map[string]string{
		"requirements.yml":          etkeccPinned,
		"upstream/requirements.yml": etkeccUpstream,
	})
	p.requireOK(p.run())
	first := p.read("requirements.yml")
	p.requireOK(p.run())
	p.requireTextEqual("requirements.yml", p.read("requirements.yml"), first)
}
