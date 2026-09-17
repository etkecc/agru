# agru

<!-- vim-markdown-toc GFM -->

* [What?](#what)
* [Why?](#why)
* [How?](#how)
* [Output](#output)
* [What's the catch?](#whats-the-catch)
    * [only git repos are supported](#only-git-repos-are-supported)
        * [same for collections](#same-for-collections)
        * [path resolution](#path-resolution)
    * [only list/update/install/remove operations are supported](#only-listupdateinstallremove-operations-are-supported)
* [Where to get?](#where-to-get)
    * [Binaries and distro-specific packages](#binaries-and-distro-specific-packages)
    * [Homebrew](#homebrew)
    * [Build yourself](#build-yourself)
* [Who uses it?](#who-uses-it)

<!-- vim-markdown-toc -->

## What?

**a**nsible-**g**alaxy **r**equirements **u**pdater. A drop-in `ansible-galaxy` replacement that's fast and doesn't argue with you:

* update requirements.yml when a newer git tag (role/collection version) shows up
* reinstall a role/collection only when its version actually changed in requirements
* install missing roles/collections
* fully backwards-compatible with `ansible-galaxy`, down to the odd trailing space it leaves in the installed roles' `meta/.galaxy_install_info`

## Why?

We at [etke.cc](https://etke.cc) maintain a pile of [Ansible roles](https://github.com/orgs/mother-of-all-self-hosting/repositories) and playbooks ([MDAD](https://github.com/spantaleev/matrix-docker-ansible-deploy), [MASH](https://github.com/mother-of-all-self-hosting/mash-playbook), [etke.cc](https://github.com/etkecc/ansible/)). We wrote A.G.R.U. because `ansible-galaxy` is slow, **very** slow. And irrational. And it skips things you'd expect it to just do:

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

Remove an installed role or collection:

```bash
$ agru -d traefik
```

That covers the daily driving. The full set of flags:

```bash
Usage of agru:
  -c	cleanup temporary files (default true)
  -cp string
    	path to install collections (default: $ANSIBLE_COLLECTIONS_PATH, then ~/.ansible/collections)
  -d string
    	delete installed role or collection, all other flags are ignored
  -i	install missing roles and collections (default true)
  -l	list installed roles and collections
  -limit int
    	limit the number of parallel downloads. 0 - no limit (default)
  -no-tui
    	deprecated, interactive mode was removed
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

## Output

agru logs plain text: one line per role, errors on stderr, everything else on stdout. If it can't write your updated `requirements.yml`, or can't create the roles directory, it says so on stderr and exits non-zero. If you're scripting agru, pipe it; that's what it's made for.

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

#### same for collections

**does** work for collections:

```yaml
collections:
  - name: git+https://github.com/ansible-collections/community.docker
    version: 3.13.0
```

**does not** work for collections:

```yaml
collections:
  - name: community.general
    version: 8.0.0
```

Collection entries that agru cannot handle (Galaxy names, version ranges, non-git types,
extra keys like `source` or `signatures`) are skipped with a one-line notice and preserved
verbatim on `-u`, exactly like roles with non-git sources.

#### path resolution

Collections are installed under `{collections_path}/ansible_collections/{namespace}/{name}/`.
The collections path is resolved in this order:

1. `-cp` flag
2. `$ANSIBLE_COLLECTIONS_PATH` env (first entry)
3. `$ANSIBLE_COLLECTIONS_PATHS` env (legacy, first entry)
4. `~/.ansible/collections` (default)

Each installed collection gets a `MANIFEST.json` written by agru so that
`ansible-galaxy collection list` can see it. agru does not resolve
transitive `dependencies` from `galaxy.yml`.

Empty version = clone HEAD, always reinstalled. `-u` never pins a version
onto an empty-version entry.

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
