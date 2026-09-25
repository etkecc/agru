//go:build e2e

package e2e

import "testing"

// Pins and sources below are copied from the consumers' own requirements.yml files.
const (
	etkeccCleanupPin = "v1.0.0-1" // one release behind upstream v1.0.0-2, so -u has something to move
	etkeccCleanupSrc = "git+https://github.com/mother-of-all-self-hosting/ansible-role-cleanup.git"
	etkeccHelpSHA    = "717de2c7fb03f8124abc2beeb34980305f723f33"
	mashActualPin    = "v26.8.1-0"
	mashActualSrc    = "git+https://iris.radicle.network/z2chD7Kt74JwEMafxTooxN7MaeYtK.git"
	mashRuntimeSHA   = "9b4b088c62b528b73a9a7c93d3109b091dd42ec6"
	mdadAuxPin       = "v1.0.0-8" // staged lag: mdad keeps all pins current, so the bump axis pins this one behind
	mdadAuxSrc       = "git+https://github.com/mother-of-all-self-hosting/ansible-role-aux.git"
	mdadDockerPin    = "8.0.0"
	mdadPreserverSHA = "dd6e15246b7a9a2d921e0b3f9cd8a4a917a1bb2f"
)

// etkecc pulls its own roles through the justfile and runs agru unattended from a schedule.
var etkeccPull = &caller{
	repo:   "etkecc/ansible",
	recipe: "justfile: pull-roles",
	source: "https://github.com/etkecc/ansible/blob/c332827935307c2163801026082edc261ac7ed43/justfile#L83-L90",
	argv:   []string{"-p", "roles/galaxy/"},
	drift: []check{{
		url:  "https://raw.githubusercontent.com/etkecc/ansible/HEAD/justfile",
		want: []string{"agru -p roles/galaxy/ ${AGRU_CLEANUP:-}", "@agru -p roles/galaxy/ {{ flags }}"},
	}},
}

// etkecc's hourly workflow is the only unattended agru caller; its installs land in the tracked roles dir.
var etkeccCI = &caller{
	repo:   "etkecc/ansible",
	recipe: "workflow: update.yml, step Update components",
	source: "https://github.com/etkecc/ansible/blob/c332827935307c2163801026082edc261ac7ed43/.github/workflows/update.yml#L66-L75",
	argv:   []string{"-u", "-p", "roles/galaxy/"},
	drift: []check{{
		url:  "https://raw.githubusercontent.com/etkecc/ansible/HEAD/.github/workflows/update.yml",
		want: []string{"agru -u -p roles/galaxy/"},
	}},
}

// mash passes an absolute requirements path, still passes the deprecated -no-tui, and hosts 104 roles on Radicle.
var mashRoles = &caller{
	repo:   "mash-playbook",
	recipe: "justfile: roles",
	source: "https://github.com/mother-of-all-self-hosting/mash-playbook/blob/38f7fce505c89510947abe60de6a28d55f0903fd/justfile#L26-L36",
	argv:   []string{"-r", "{{dir}}/requirements.yml", "-p", "roles/galaxy/", "-no-tui"},
	drift: []check{{
		url: "https://raw.githubusercontent.com/mother-of-all-self-hosting/mash-playbook/HEAD/justfile",
		want: []string{
			"agru -r {{ justfile_directory() }}/requirements.yml -p roles/galaxy/ -no-tui",
			"agru -r {{ templates_directory_path }}/requirements.yml -p roles/galaxy/ -no-tui {{ flags }}",
		},
	}},
}

// mash's update recipe reads the generated-from template file instead of the root one.
var mashUpdate = &caller{
	repo:   "mash-playbook",
	recipe: "justfile: update",
	source: "https://github.com/mother-of-all-self-hosting/mash-playbook/blob/38f7fce505c89510947abe60de6a28d55f0903fd/justfile#L91-L101",
	argv:   []string{"-r", "{{dir}}/templates/requirements.yml", "-p", "roles/galaxy/", "-no-tui"},
}

// mdad relies on the default requirements.yml lookup, and pins four tagless roles by sha.
var mdadRoles = &caller{
	repo:   "matrix-docker-ansible-deploy",
	recipe: "justfile: roles",
	source: "https://github.com/spantaleev/matrix-docker-ansible-deploy/blob/f9b4a7f071dc85737a500a1d842d9688c0d04696/justfile#L22-L30",
	argv:   []string{"-p", "roles/galaxy/", "-no-tui"},
	drift: []check{{
		url: "https://raw.githubusercontent.com/spantaleev/matrix-docker-ansible-deploy/HEAD/justfile",
		want: []string{
			"agru -p roles/galaxy/ -no-tui",
			"agru -p roles/galaxy/ -no-tui {{ flags }}",
		},
	}},
}

// mdad's update recipe is the same argv with the flags its users pass, `-u` among them.
var mdadUpdate = &caller{
	repo:   "matrix-docker-ansible-deploy",
	recipe: "justfile: update",
	source: "https://github.com/spantaleev/matrix-docker-ansible-deploy/blob/f9b4a7f071dc85737a500a1d842d9688c0d04696/justfile#L31-L42",
	argv:   []string{"-p", "roles/galaxy/", "-no-tui"},
}

// etkeccPinned is etkecc's requirements.yml shape: list form, an include of the mdad submodule, tag and main pins.
const etkeccPinned = `---

- include: upstream/requirements.yml
- src: git+https://github.com/mother-of-all-self-hosting/ansible-role-cleanup.git
  version: v1.0.0-1
  name: cleanup
- src: git+https://github.com/mother-of-all-self-hosting/ansible-role-fail2ban.git
  version: v1.0.0-3
  name: fail2ban
- src: git+https://github.com/mother-of-all-self-hosting/ansible-role-swap.git
  version: main
  name: swap
`

// etkeccUpstream stands in for etkecc's upstream/ submodule: mdad's shapes, sha pin and .git-less src included.
const etkeccUpstream = `---

- src: git+https://github.com/devture/com.devture.ansible.role.playbook_help.git
  version: 717de2c7fb03f8124abc2beeb34980305f723f33
  name: playbook_help
- src: git+https://github.com/geerlingguy/ansible-role-docker
  version: 8.0.0
  name: docker
`

// mashPinned is mash's requirements.yml shape: Radicle host, activation_prefix extras, an empty prefix, a sha pin.
const mashPinned = `---

- src: git+https://iris.radicle.network/z2chD7Kt74JwEMafxTooxN7MaeYtK.git
  version: v26.8.1-0
  name: actual
  activation_prefix: actual_
- src: git+https://github.com/devture/com.devture.ansible.role.playbook_runtime_messages.git
  version: 9b4b088c62b528b73a9a7c93d3109b091dd42ec6
  name: playbook_runtime_messages
  activation_prefix: ""
`

// mdadPinned is mdad's requirements.yml shape: a lagging tag pin, a .git-less src, a sha pin on a tagless repo.
const mdadPinned = `---

- src: git+https://github.com/mother-of-all-self-hosting/ansible-role-aux.git
  version: v1.0.0-8
  name: auxiliary
- src: git+https://github.com/geerlingguy/ansible-role-docker
  version: 8.0.0
  name: docker
- src: git+https://github.com/devture/com.devture.ansible.role.playbook_state_preserver.git
  version: dd6e15246b7a9a2d921e0b3f9cd8a4a917a1bb2f
  name: playbook_state_preserver
`

// TestEtkeccPullRoles runs `just pull-roles`: every pin installs, include expansion included.
func TestEtkeccPullRoles(t *testing.T) {
	p := newProject(t, etkeccPull, map[string]string{
		"requirements.yml":          etkeccPinned,
		"upstream/requirements.yml": etkeccUpstream,
	})
	p.requireOK(p.run())
	p.requireInstalled("cleanup", etkeccCleanupPin)
	p.requireInstalled("fail2ban", "v1.0.0-3")
	p.requireInstalled("swap", "main")
	p.requireInstalled("playbook_help", etkeccHelpSHA)
	p.requireInstalled("docker", mdadDockerPin)
}

// TestEtkeccCIUpdate runs the hourly step: -u moves the lagging pin only, and never touches the include file.
func TestEtkeccCIUpdate(t *testing.T) {
	p := newProject(t, etkeccCI, map[string]string{
		"requirements.yml":          etkeccPinned,
		"upstream/requirements.yml": etkeccUpstream,
	})
	before := p.read("requirements.yml")
	bumped := newestTag(t, etkeccCleanupSrc)
	if bumped == etkeccCleanupPin {
		t.Fatalf("fixture is no longer behind: pin %s at an older tag of %s", etkeccCleanupPin, etkeccCleanupSrc)
	}
	p.requireOK(p.run())
	p.requireTextEqual("requirements.yml", p.read("requirements.yml"), replacePin(before, etkeccCleanupPin, bumped))
	p.requireTextEqual("upstream/requirements.yml", p.read("upstream/requirements.yml"), etkeccUpstream)
	p.requireInstalled("cleanup", bumped)
	p.requireInstalled("swap", "main")
	p.requireInstalled("playbook_help", etkeccHelpSHA)
}

// TestMashJustRoles runs `just roles`: absolute -r, the deprecated -no-tui, and a Radicle-hosted role.
func TestMashJustRoles(t *testing.T) {
	p := newProject(t, mashRoles, map[string]string{"requirements.yml": mashPinned})
	p.requireOK(p.run())
	p.requireInstalled("actual", mashActualPin)
	p.requireInstalled("playbook_runtime_messages", mashRuntimeSHA)
}

// TestMashJustUpdate runs `just update -u`: the Radicle tag pin moves, the tagless sha pin does not.
func TestMashJustUpdate(t *testing.T) {
	p := newProject(t, mashUpdate, map[string]string{"templates/requirements.yml": mashPinned})
	before := p.read("templates/requirements.yml")
	bumped := newestTag(t, mashActualSrc)
	p.requireOK(p.run("-u"))
	p.requireTextEqual("templates/requirements.yml", p.read("templates/requirements.yml"), replacePin(before, mashActualPin, bumped))
	p.requireInstalled("actual", bumped)
	p.requireInstalled("playbook_runtime_messages", mashRuntimeSHA)
}

// TestMdadJustRoles runs `just roles`: default requirements.yml lookup plus -no-tui.
func TestMdadJustRoles(t *testing.T) {
	p := newProject(t, mdadRoles, map[string]string{"requirements.yml": mdadPinned})
	p.requireOK(p.run())
	p.requireInstalled("auxiliary", mdadAuxPin)
	p.requireInstalled("docker", mdadDockerPin)
	p.requireInstalled("playbook_state_preserver", mdadPreserverSHA)
}

// TestMdadJustUpdate runs `just update -u`: the lagging pin moves, the newest pin and the sha pin do not.
func TestMdadJustUpdate(t *testing.T) {
	p := newProject(t, mdadUpdate, map[string]string{"requirements.yml": mdadPinned})
	before := p.read("requirements.yml")
	bumped := newestTag(t, mdadAuxSrc)
	if bumped == mdadAuxPin {
		t.Fatalf("fixture is no longer behind: pin %s at an older tag of %s", mdadAuxPin, mdadAuxSrc)
	}
	p.requireOK(p.run("-u"))
	p.requireTextEqual("requirements.yml", p.read("requirements.yml"), replacePin(before, mdadAuxPin, bumped))
	p.requireInstalled("auxiliary", bumped)
	p.requireInstalled("docker", mdadDockerPin)
	p.requireInstalled("playbook_state_preserver", mdadPreserverSHA)
}
