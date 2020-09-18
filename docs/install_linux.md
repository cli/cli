# Installing gh on Linux

Packages downloaded from https://cli.github.com or from https://github.com/cli/cli/releases
are considered official binaries. We focus on a couple of popular Linux distros and
the following CPU architectures: `i386`, `amd64`, `arm64`.

Other sources for installation are community-maintained and thus might lag behind
our release schedule.

## Official sources

### Debian, Ubuntu Linux (apt)

Install:

```bash
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-key C99B11DEB97541F0
sudo apt-add-repository https://cli.github.com/packages
sudo apt update
sudo apt install gh
```

**Note**: most systems will have `apt-add-repository` already. If you get a _command not found_
error, try running `sudo apt install software-properties-common` and trying these steps again.

Upgrade:

```bash
sudo apt update
sudo apt install gh
```

### Fedora, Centos, Red Hat Linux (dnf)

Install:

```bash
sudo dnf config-manager --add-repo https://cli.github.com/packages/rpm/gh-cli.repo
sudo dnf install gh
```

Upgrade:

```bash
sudo dnf update gh
```

### openSUSE/SUSE Linux (zypper)

Install:

```bash
sudo zypper addrepo https://cli.github.com/packages/rpm/gh-cli.repo
sudo zypper ref
sudo zypper install gh
```

Upgrade:

```bash
sudo zypper ref
sudo zypper update gh
```

## Manual installation

* [Download release binaries][releases page] that match your platform; or
* [Build from source](./source.md).

### openSUSE/SUSE Linux (zypper)
 
Install and upgrade:

1. Download the `.rpm` file from the [releases page][];
2. Install the downloaded file: `sudo zypper in gh_*_linux_amd64.rpm`

## Community-supported methods

Our team does not directly maintain the following packages or repositories.

### Arch Linux

Arch Linux users can install from the [community repo][arch linux repo]:

```bash
sudo pacman -S github-cli
```

### Android

Android users can install via Termux:

```bash
pkg install gh
```

### Kiss Linux

Kiss Linux users can install from the [community repos](https://github.com/kisslinux/community):

```bash
kiss b github-cli && kiss i github-cli
```


[releases page]: https://github.com/cli/cli/releases/latest
[arch linux repo]: https://www.archlinux.org/packages/community/x86_64/github-cli
