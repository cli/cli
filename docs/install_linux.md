# Installing gh on Linux and BSD

## Recommended _(Official)_

### Debian

Debian packages are hosted on the [GitHub CLI marketing site](https://cli.github.com/) for various operating systems including:

- [Debian](https://www.debian.org/)
- [Raspberry Pi](https://www.raspberrypi.com/)
- [Ubuntu Linux](https://ubuntu.com/)

These packages are supported by the GitHub CLI maintainers with updates powered by [GitHub CLI deployment workflow](https://github.com/cli/cli/actions/workflows/deployment.yml).

To install:

```bash
(type -p wget >/dev/null || (sudo apt update && sudo apt install wget -y)) \
	&& sudo mkdir -p -m 755 /etc/apt/keyrings \
	&& out=$(mktemp) && wget -nv -O$out https://cli.github.com/packages/githubcli-archive-keyring.gpg \
	&& cat $out | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null \
	&& sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
	&& sudo mkdir -p -m 755 /etc/apt/sources.list.d \
	&& echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
	&& sudo apt update \
	&& sudo apt install gh -y
```

To upgrade:

```bash
sudo apt update
sudo apt install gh
```

### RPM

RPM packages are hosted on the [GitHub CLI marketing site](https://cli.github.com) for various operating systems including:

- [Amazon Linux 2](https://aws.amazon.com/amazon-linux-2/)
- [CentOS](https://www.centos.org/)
- [Fedora](https://fedoraproject.org/)
- [Red Hat Enterprise Linux](https://www.redhat.com/en/technologies/linux-platforms/enterprise-linux)
- [openSUSE](https://www.opensuse.org/)
- [SUSE](https://www.suse.com/)

These packages are supported by the GitHub CLI maintainers with updates powered by [GitHub CLI deployment workflow](https://github.com/cli/cli/actions/workflows/deployment.yml).

#### DNF5

> [!IMPORTANT]
> **These commands apply to DNF5 only**. If you're using DNF4, please use [the DNF4 instructions](#dnf4).

To install:

```bash
sudo dnf install dnf5-plugins
sudo dnf config-manager addrepo --from-repofile=https://cli.github.com/packages/rpm/gh-cli.repo
sudo dnf install gh --repo gh-cli
```

To upgrade:

```bash
sudo dnf update gh
```

#### DNF4

> [!IMPORTANT]
> **These commands apply to DNF4 only**. If you're using DNF5, please use [the DNF5 instructions](#dnf5).

To install:

```bash
sudo dnf install 'dnf-command(config-manager)'
sudo dnf config-manager --add-repo https://cli.github.com/packages/rpm/gh-cli.repo
sudo dnf install gh --repo gh-cli
```

To upgrade:

```bash
sudo dnf update gh
```

#### Amazon Linux 2 (yum)

To install:

```bash
type -p yum-config-manager >/dev/null || sudo yum install yum-utils
sudo yum-config-manager --add-repo https://cli.github.com/packages/rpm/gh-cli.repo
sudo yum install gh
```

To upgrade:

```bash
sudo yum update gh
```

#### openSUSE/SUSE Linux (zypper)

To install:

```bash
sudo zypper addrepo https://cli.github.com/packages/rpm/gh-cli.repo
sudo zypper ref
sudo zypper install gh
```

To upgrade:

```bash
sudo zypper ref
sudo zypper update gh
```

### Homebrew

[Homebrew](https://brew.sh/) is a free and open-source software package management system that simplifies the installation of software on Apple's operating system, macOS, as well as Linux.

The [GitHub CLI formulae](https://formulae.brew.sh/formula/gh) is supported by the GitHub CLI maintainers with help from our friends at Homebrew with updated powered by [homebrew/hoomebrew-core](https://github.com/Homebrew/homebrew-core/blob/main/Formula/g/gh.rb).

To install:

```shell
brew install gh
```

To upgrade:

```shell
brew upgrade gh
```

### Precompiled binaries

[GitHub CLI releases](https://github.com/cli/cli/releases/latest) contain precompiled binaries for `386`, `amd64`, `arm64`, and `armv6` architectures.

## Community _(Unofficial)_

> [!IMPORTANT]
> The GitHub CLI team does not maintain the following packages or repositories. We are unable to provide support for these installation methods or any guarantees of stability, security, or availability for these installation methods.

### Alpine Linux

The [GitHub CLI package](https://pkgs.alpinelinux.org/package/edge/community/x86_64/github-cli) is supported by the Alpine Linux community with updates powered by [alpine/aports](https://gitlab.alpinelinux.org/alpine/aports/-/tree/master/community/github-cli).

To install stable release:

```bash
apk add github-cli
```

To install edge release:

```bash
echo "@community http://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories
apk add github-cli@community
```

### Android

The [GitHub CLI package](https://packages.termux.dev/apt/termux-main/pool/main/g/gh/) is supported by the Termux community with updates powered by [termux/termux-packages](https://github.com/termux/termux-packages/tree/master/packages/gh).

To install and upgrade:

```bash
pkg install gh
```

### Arch Linux

The [GitHub CLI package](https://www.archlinux.org/packages/extra/x86_64/github-cli) is supported by the Arch Linux community with updates powered by [Arch Linux packaging](https://gitlab.archlinux.org/archlinux/packaging/packages/github-cli).

To install:

```bash
sudo pacman -S github-cli
```

To upgrade all packages:

```bash
sudo pacman -Syu
```

Alternatively, use the [unofficial AUR package](https://aur.archlinux.org/packages/github-cli-git) to build GitHub CLI from source.

### Conda

[Conda](https://docs.conda.io/en/latest/) is an open source package management system and environment management system for installing multiple versions of software packages and their dependencies and switching easily between them. It works on Linux, OS X and Windows, and was created for Python programs but can package and distribute any software.

The [GitHub CLI package](https://anaconda.org/conda-forge/gh) is supported by the Conda community with updates powered by [conda-forge/gh-feedstock](https://github.com/conda-forge/gh-feedstock#installing-gh).

To install:

```shell
conda install gh --channel conda-forge
```

To upgrade:

```shell
conda update gh --channel conda-forge
```

### Debian Community

The [GitHub CLI package](https://packages.debian.org/stable/gh) is supported by the Debian community with updates powered by [Debian Go Packaging Team](https://salsa.debian.org/go-team/packages/gh).

> [!NOTE]
> As of November 2025, GitHub CLI maintainers strongly recommend [official Debian packages](#debian) especially as the community-distributed `2.45.x` / `2.46.x` version is broken due to deprecated GitHub APIs.

### Fedora Community

The [GitHub CLI package](https://packages.fedoraproject.org/pkgs/gh/gh/) is supported by the Fedora community with updates powered by [Fedora Project](https://src.fedoraproject.org/rpms/gh).

To install:

```bash
sudo dnf install gh
```

To upgrade:

```bash
sudo dnf update gh
```

### Flox

[Flox](https://flox.dev/) is a virtual environment and package manager all in one. With Flox you create environments that layer and replace dependencies just where it matters, making them portable across the full software lifecycle.

Flox relies upon the [GitHub CLI package](https://github.com/NixOS/nixpkgs/blob/master/pkgs/by-name/gh/gh/package.nix) supported by the [NixOS community](https://nixos.org/)

To install:

```shell
flox install gh
```

To upgrade:

```shell
flox upgrade toplevel
```

### FreeBSD

The [GitHub CLI port](https://www.freshports.org/devel/gh/) is supported by the FreeBSD community with updates powered by [FreeBSD ports](https://cgit.freebsd.org/ports/tree/devel/gh).

```bash
cd /usr/ports/devel/gh/ && make install clean
```

Or via [pkg(8)](https://www.freebsd.org/cgi/man.cgi?pkg(8)):

```bash
pkg install gh
```

### Funtoo

The GitHub CLI portage is supported by the Funtoo community with updates powered by [funtoo/dev-kit](https://github.com/funtoo/dev-kit/tree/1.4-release/dev-util/github-cli).

To install:

```bash
emerge -av github-cli
```

To upgrade:

```bash
ego sync
emerge -u github-cli
```

### Gentoo

The [GitHub CLI portage](https://packages.gentoo.org/packages/dev-util/github-cli) is supported by the Gentoo community with updates powered by [Gentoo portage](https://gitweb.gentoo.org/repo/gentoo.git/tree/dev-util/github-cli).

To install:

``` bash
emerge -av github-cli
```

To upgrade:

``` bash
emerge --sync
emerge -u github-cli
```

### Manjaro Linux

The [GitHub CLI package](https://manjaristas.org/branch_compare?q=github-cli) is the same package produced by the [Arch Linux community](#arch-linux)

To install and upgrade:

```bash
pamac install github-cli
```

### MidnightBSD

The [GitHub CLI port](https://www.midnightbsd.org/mports/devel/gh/README.html) is supported by the MidnightBSD community with updates powered by [MidnightBSD/mports](https://github.com/MidnightBSD/mports/tree/master/devel/gh).

To install:

```bash
cd /usr/mports/devel/gh/ && make install clean
```

Or via [mport(1)](http://man.midnightbsd.org/cgi-bin/man.cgi/mport):

```bash
mport install gh
```

### NetBSD/pkgsrc

The [GitHub CLI package](https://pkgsrc.se/net/gh) is supported by the NetBSD community with updates powered by [NetBSD/pkgsrc](https://github.com/NetBSD/pkgsrc/tree/trunk/net/gh).

To install:

```bash
pkgin install gh
```

### Nix/NixOS

The [GitHub CLI package](https://search.nixos.org/packages?query=gh&sort=relevance&show=gh) is supported by the NixOS community with updates powered by [NixOS/nixpkgs](https://github.com/NixOS/nixpkgs/tree/master/pkgs/by-name/gh/gh).

To install:

```bash
nix-env -iA nixos.gh
```

### OpenBSD

The [GitHub CLI port](https://openports.pl/path/devel/github-cli) is supported by the OpenBSD community with updates powered by [OpenBSD ports](https://cvsweb.openbsd.org/ports/devel/github-cli/).

To install:

```shell
pkg_add github-cli
```

### openSUSE Tumbleweed

The [GitHub CLI package](https://software.opensuse.org/package/gh) is supported by the openSUSE community.

To install:

```bash
sudo zypper install gh
```

To upgrade:

```bash
sudo zypper update gh
```

### Solus Linux

The GitHub CLI package is supported by the Solus Linux community with updates powered by [getsolus/packages](https://github.com/getsolus/packages/blob/main/packages/g/github-cli/).

To install:

```bash
sudo eopkg install github-cli
```

### Spack

[Spack](https://spack.io/) is a flexible package manager supporting multiple versions, configurations, platforms, and compilers for supercomputers, Linux, and macOS.

The [GitHub CLI package](https://packages.spack.io/package.html?name=gh) is supported by the Spack community with updates powered by [spack/spack-packages](https://github.com/spack/spack-packages/tree/develop/repos/spack_repo/builtin/packages/gh).

To install:

```shell
spack install gh
```

To upgrade:

```shell
spack uninstall gh && spack install gh
```

### Ubuntu Community

The [GitHub CLI package](https://packages.ubuntu.com/noble/gh) is synced from [upstream Debian Community package](#debian-community).

> [!NOTE]
> As of November 2025, GitHub CLI maintainers strongly recommend [official Debian packages](#debian) especially as the community-distributed `2.45.x` / `2.46.x` version is broken due to deprecated GitHub APIs.

### Void Linux

The [GitHub CLI package](https://voidlinux.org/packages/?arch=x86_64&q=github-cli): is supported by the Void Linux community with updates powered by [void-linux/void-packages](https://github.com/void-linux/void-packages/tree/master/srcpkgs/github-cli).

To install:

```bash
sudo xbps-install github-cli
```

### Webi

[Webi](https://webinstall.dev/) is a tool that aims to effortlessly install developer tools with easy-to-remember URLs from official builds quickly, without sudo or Admin, without a package manager, and without changing system file permissions.

The [GitHub CLI package](https://webinstall.dev/gh/) is supported by the Webi community with updates powered by [webinstall/webi-installers](https://github.com/webinstall/webi-installers/tree/main/gh).

To install:

```shell
curl -sS https://webi.sh/gh \| sh
```

To upgrade:

```shell
webi gh@stable
```

## Discouraged

> [!WARNING]
> The GitHub CLI team actively discourages use of the following methods of installation.

### Snap

The [GitHub CLI package](https://snapcraft.io/gh) has [so many issues with Snap](https://github.com/casperdcl/cli/issues/7) as a runtime mechanism for apps like GitHub CLI that our team suggests _never installing gh as a snap_.
