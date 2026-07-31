# agru

<!-- vim-markdown-toc GFM -->

* [What?](#what)
* [Why?](#why)
* [How?](#how)
* [Non-interactive mode](#non-interactive-mode)
* [What's the catch?](#whats-the-catch)
    * [only git repos are supported](#only-git-repos-are-supported)
    * [only roles are supported](#only-roles-are-supported)
    * [only list/update/install/remove operations are supported](#only-listupdateinstallremove-operations-are-supported)
* [Where to get?](#where-to-get)
    * [Binaries and distro-specific packages](#binaries-and-distro-specific-packages)
    * [Homebrew](#homebrew)
    * [Build yourself](#build-yourself)
* [Who uses it?](#who-uses-it)

<!-- vim-markdown-toc -->

## What?

**a**nsible-**g**alaxy **r**equirements **u**pdater. A drop-in `ansible-galaxy` replacement that's fast and doesn't argue with you:

* update requirements.yml when a newer git tag (role version) shows up
* reinstall a role only when its version actually changed in requirements
* install missing roles
* fully backwards-compatible with `ansible-galaxy`, down to the odd trailing space it leaves in the installed roles' `meta/.galaxy_install_info`

## Why?

We at [etke.cc](https://etke.cc) maintain a pile of [Ansible roles](https://github.com/orgs/mother-of-all-self-hosting/repositories) and playbooks ([MDAD](https://github.com/spantaleev/matrix-docker-ansible-deploy), [MASH](https://github.com/mother-of-all-self-hosting/mash-playbook), [etke.cc](https://gitlab.com/etke.cc/ansible/)). We wrote A.G.R.U. because `ansible-galaxy` is slow, **very** slow. And irrational. And it skips things you'd expect it to just do:

* Bumped a role's version in requirements? `ansible-galaxy install -r requirements.yml -p roles/galaxy/` won't install it. You get to add `--force` or delete the dir by hand. agru just does it.
* Got 100500 roles and want to know which ones have a newer tag? Checking by hand is your evening gone. agru checks all of them at once.
* Installs drag on forever? agru does the same work in a fraction of the time.

It started as a maintainer's tool, then turned out to be useful for everyone. Our playbooks ship a `just update` command (maintainers: `just update -u`) that updates the playbook and installs every role. And it's fast.

## How?

Most of the time you just run it. Install everything missing from your requirements file:

```bash
$ agru
```

List what's installed:

```bash
$ agru -l
```

Update requirements to the newest available tags:

```bash
$ agru -u
```

Remove an installed role:

```bash
$ agru -d traefik
```

That covers the daily driving. The full set of flags:

```bash
Usage of agru:
  -c	cleanup temporary files (default true)
  -d string
    	delete installed role, all other flags are ignored
  -i	install missing roles (default true)
  -k	keep TUI open after completion until 'q'
  -l	list installed roles
  -limit int
    	limit the number of parallel downloads (affects roles installation only). 0 - no limit (default)
  -no-tui
    	force non-interactive logging output (no TUI)
  -p string
    	path to install roles (default "roles/galaxy/")
  -r string
    	ansible-galaxy requirements file (default "requirements.yml")
  -u	update requirements file if newer versions are available
  -v	print version and exit
  -verbose
    	verbose output
  -version
    	print version and exit
```

## Non-interactive mode

agru draws a TUI when it has a terminal to draw on. Pipe it, redirect it, or run it in CI, and that TUI just garbles your log with escape codes. So when stdout isn't a terminal, agru skips the TUI and logs plain text instead: one line per role, errors on stderr, everything else on stdout.

Want the plain output from a real terminal too? Force it:

```bash
$ agru -no-tui
```

One difference from the TUI, and it's in your favor: the plain path checks whether things actually worked. If it can't write your updated `requirements.yml`, or can't create the roles directory, it says so on stderr and exits non-zero. The TUI swallows both and exits like nothing happened. If you're scripting agru, this is the mode you want.

## What's the catch?

Think A.G.R.U. is too good to be true? It's true, it just has limits:

### only git repos are supported

does **not** work:

```yaml
- src: geerlingguy.docker
  version: 6.1.0
```

**does** work:
```yaml
- src: git+https://github.com/geerlingguy/ansible-role-docker
  name: geerlingguy.docker
  version: 6.1.0
```

### only roles are supported

No collections at this moment, at all.

### only list/update/install/remove operations are supported

Ansible Galaxy API is not used at all, thus no API-related actions are supported

## Where to get?

### Binaries and distro-specific packages

[Releases page](https://github.com/etkecc/agru/releases) and [Arch Linux AUR](https://aur.archlinux.org/packages/agru)

### Homebrew

agru isn't in homebrew-core (it's a small tool, well under the notability bar for that), so it ships as its own tap instead. Since the tap lives in this repo rather than in a separate `homebrew-agru` repo, the full tap URL is required:

```bash
$ brew tap etkecc/agru https://github.com/etkecc/agru
$ brew install agru
```

### Build yourself

`just build` or `go build .`

## Who uses it?

- [Matrix Docker Ansible Deploy (MDAD)](https://github.com/spantaleev/matrix-docker-ansible-deploy)
- [Mother of All Self-Hosting (MASH)](https://github.com/mother-of-all-self-hosting/mash-playbook)
- [etke.cc](https://github.com/etkecc/ansible)

If you use A.G.R.U. in your project, please let us know by creating an issue or PR with your project link.
