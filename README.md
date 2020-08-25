# GitHub CLI

`gh` is GitHub on the command line, and it's now available in beta. It brings pull requests, issues, and other GitHub concepts to
the terminal next to where you are already working with `git` and your code.

![screenshot of gh pr status](https://user-images.githubusercontent.com/98482/84171218-327e7a80-aa40-11ea-8cd1-5177fc2d0e72.png)

## Availability

While in beta, GitHub CLI is available for repos hosted on GitHub.com only. It currently does not support repositories hosted on GitHub Enterprise Server or other hosting providers. We are planning on adding support for GitHub Enterprise Server after GitHub CLI is out of beta (likely towards the end of 2020), and we want to ensure that the API endpoints we use are more widely available for GHES versions that most GitHub customers are on.

## We need your feedback

GitHub CLI is currently in its early development stages, and we're hoping to get feedback from people using it.

If you've installed and used `gh`, we'd love for you to take a short survey here (no more than five minutes): https://forms.gle/umxd3h31c7aMQFKG7

And if you spot bugs or have features that you'd really like to see in `gh`, please check out the [contributing page][]

## Usage

- `gh pr [status, list, view, checkout, create]`
- `gh issue [status, list, view, create]`
- `gh repo [view, create, clone, fork]`
- `gh config [get, set]`
- `gh help`

## Documentation

Read the [official docs](https://cli.github.com/manual/) for more information.

## Comparison with hub

For many years, [hub][] was the unofficial GitHub CLI tool. `gh` is a new project that helps us explore
what an official GitHub CLI tool can look like with a fundamentally different design. While both
tools bring GitHub to the terminal, `hub` behaves as a proxy to `git`, and `gh` is a standalone
tool. Check out our [more detailed explanation](/docs/gh-vs-hub.md) to learn more.


<!-- this anchor is linked to from elsewhere, so avoid renaming it -->
## Installation

### macOS

`gh` is available via Homebrew and MacPorts.

#### Homebrew

Install:

```bash
brew install github/gh/gh
```

Upgrade:

```bash
brew upgrade gh
```

#### MacPorts

Install:

```bash
sudo port install gh
```

Upgrade:

```bash
sudo port selfupdate && sudo port upgrade gh
```

### Windows

`gh` is available via [scoop][], [Chocolatey][], and as downloadable MSI.

#### scoop

Install:

```powershell
scoop bucket add github-gh https://github.com/cli/scoop-gh.git
scoop install gh
```

Upgrade:

```powershell
scoop update gh
```

#### Chocolatey

Install:

```powershell
choco install gh
```

Upgrade:

```powershell
choco upgrade gh
```

#### Signed MSI

MSI installers are available for download on the [releases page][].

### Debian/Ubuntu Linux

Install and upgrade:

1. Download the `.deb` file from the [releases page][];
2. Install the downloaded file: `sudo apt install ./gh_*_linux_amd64.deb`

### Fedora Linux

Install and upgrade:

1. Download the `.rpm` file from the [releases page][];
2. Install the downloaded file: `sudo dnf install gh_*_linux_amd64.rpm`

### Centos Linux

Install and upgrade:

1. Download the `.rpm` file from the [releases page][];
2. Install the downloaded file: `sudo yum localinstall gh_*_linux_amd64.rpm` 

### openSUSE/SUSE Linux

Install and upgrade:

1. Download the `.rpm` file from the [releases page][];
2. Install the downloaded file: `sudo zypper in gh_*_linux_amd64.rpm`

### Arch Linux

Arch Linux users can install from the [community repo](https://www.archlinux.org/packages/community/x86_64/github-cli/):

```bash
pacman -S github-cli
```

### Android

Android users can install via Termux:

```bash
pkg install gh
```

### Other platforms

Download packaged binaries from the [releases page][].

### Build from source

See here on how to [build GitHub CLI from source](/docs/source.md).

[docs]: https://cli.github.com/manual
[scoop]: https://scoop.sh
[Chocolatey]: https://chocolatey.org
[releases page]: https://github.com/cli/cli/releases/latest
[hub]: https://github.com/github/hub
[contributing page]: https://github.com/cli/cli/blob/trunk/.github/CONTRIBUTING.md
