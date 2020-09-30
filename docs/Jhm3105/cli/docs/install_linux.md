# Installing gh on Linux

Packages downloaded from https://cli.github.com or from https://github.com/cli/cli/releases
are considered official binaries. We focus on popular Linux distros and
the following CPU architectures: `i386`, `amd64`, `arm64`.

Other sources for installation are community-maintained and thus might lag behind
our release schedule.

If none of our official binaries, packages, repositories, nor community sources work for you, we recommend using our `Makefile` to build `gh` from source. It's quick and easy.

## Official sources

### Debian, Ubuntu Linux (apt)

Install:

```bash
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-key C99B11DEB97541F0
sudo apt-add-repository https://cli.github.com/packages
sudo apt update
sudo apt install gh
```

**Note**: If you are behind a firewall, the connection to `keyserver.ubuntu.com` might fail. In that case, try running `sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-key C99B11DEB97541F0`.

**Note**: If you get _"gpg: failed to start the dirmngr '/usr/bin/dirmngr': No such file or directory"_ error, try installing the `dirmngr` package. Run `sudo apt-get install dirmngr` and repeat the steps above.  

**Note**: most systems will have `apt-add-repository` already. If you get a _command not found_
error, try running `sudo apt install software-properties-common` and trying these steps again.


Upgrade:

```bash
sudo apt update
sudo apt install gh
```

### Fedora, CentOS, Red Hat Enterprise Linux (dnf)

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

Our team does not directly maintain the following packages or repositories. They are unofficial and we are unable to provide support or guarantees for them.

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

### Nix/NixOS

Nix/NixOS users can install from [nixpkgs](https://search.nixos.org/packages?show=gitAndTools.gh&query=gh&from=0&size=30&sort=relevance&channel=20.03#disabled):

```bash
nix-env -iA nixos.gitAndTools.gh
```


[releases page]: https://github.com/cli/cli/releases/latest
[arch linux repo]: https://www.archlinux.org/packages/community/x86_64/github-cli
