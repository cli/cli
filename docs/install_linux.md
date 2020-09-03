# Installing gh on Linux

The core, paid developers of `gh` officially support a `.deb` repository and a `.rpm` repository. We
primarily test against Ubuntu and Fedora but do our best to support other distros that can work with
our repositories. We focus on support for `amd64` and `i386` architectures.

All other combinations of distro, packaging, or architecture should be considered community
supported.

## Official methods

### Debian/Ubuntu Linux (apt)

Install:

```bash
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-key C99B11DEB97541F0
sudo apt-add-repository -u https://cli.github.com/packages
sudo apt install gh
```

Upgrade:

```
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
sudo dnf install gh
```

## Community supported methods

### openSUSE/SUSE Linux

It's possible that https://cli.github.com/packages/rpm/gh-cli.repo will work with zypper but it
hasn't been tested. Otherwise, to install from package:
 
Install and upgrade:

1. Download the `.rpm` file from the [releases page][];
2. Install the downloaded file: `sudo zypper in gh_*_linux_amd64.rpm`

### Arch Linux

Arch Linux users can install from the [community repo][arch linux repo]:

```bash
pacman -S github-cli
```

### Android

Android users can install via Termux:

```bash
pkg install gh
```

[arch linux repo]: https://www.archlinux.org/packages/community/x86_64/github-cli
