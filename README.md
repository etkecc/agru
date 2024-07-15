# agru

<!-- vim-markdown-toc GitLab -->

* [What?](#what)
* [Why?](#why)
* [How?](#how)
* [What's the catch?](#whats-the-catch)
    * [only git repos are supported](#only-git-repos-are-supported)
    * [only roles are supported](#only-roles-are-supported)
    * [only list/update/install/remove operations are supported](#only-listupdateinstallremove-operations-are-supported)
* [Where to get?](#where-to-get)
    * [Binaries and distro-specific packages](#binaries-and-distro-specific-packages)
    * [Build yourself](#build-yourself)

<!-- vim-markdown-toc -->

## What?

**a**nsible-**g**alaxy **r**equirements **u**pdater is fast ansible-galaxy replacement with the following features:

* update requirements.yml file if a newer git tag (role version) is available
* update installed roles only when new version is present in requirements file
* install missing roles
* full backwards-compatibility with `ansible-galaxy`, yes, even the odd trailing space in the galaxy-installed roles' meta/.galaxy_install_info is present

## Why?

Because `ansible-galaxy` is slow, **very** slow. And irrational. And miss some functions.

* You updated some role's version in requirements file? Sorry, `ansible-galaxy install -r requirements.yml -p roles/galaxy/` can't install it, you have to use `--force` or remove the dir manually. A.G.R.U. does that automatically
* You have 100500 roles in your requirements file and you have to manually check each of them if a newer tag is available? A.G.R.U. does that automatically
* Roles installation takes ages with `ansible-galaxy`? A.G.R.U. needs a fraction of that time to install everything

## How?

```bash
Usage of agru:
  -c	cleanup temporary files (default true)
  -d string
    	delete installed role, all other flags are ignored
  -i	install missing roles (default true)
  -l	list installed roles
  -p string
    	path to install roles (default "roles/galaxy/")
  -r string
    	ansible-galaxy requirements file (default "requirements.yml")
  -u	update requirements file if newer versions are available
  -v	verbose output
```

**list installed roles**

```bash
$ agru -l
```

**install role from the requirements file**

```bash
$ agru
```

**update requirements file if newer versions are available**

```bash
$ agru -u
```

**remove already installed role**

```bash
$ agru -d traefik
```

## What's the catch?

Do you think A.G.R.U. is too good to be true? Well, it's true, but it has limitations:

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

[Releases page](https://gitlab.com/etke.cc/tools/agru/-/releases) and [Arch Linux AUR](https://aur.archlinux.org/packages/agru)

### Build yourself

`just build` or `go build .`
