# agru

<!-- vim-markdown-toc GitLab -->

* [What?](#what)
* [Why?](#why)
* [How?](#how)
* [What's a catch?](#whats-a-catch)
  * [only git repos are supported](#only-git-repos-are-supported)
  * [only roles are supported](#only-roles-are-supported)
  * [only update/install operations are supported](#only-updateinstall-operations-are-supported)

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

```
Usage of agru:
  -p string
    	path to install roles (default "roles/galaxy/")
  -r string
    	ansible-galaxy requirements file (default "requirements.yml")
  -u	update requirements file if newer versions are available
```

## What's a catch?

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

### only update/install operations are supported

No list, no role upload to galaxy, no role removal from galaxy.
In fact, galaxy API is not used at all, thus no API-related actions are supported
