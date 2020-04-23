# GitHub CLI

`gh` is GitHub on the command line, and it's now available in beta. It brings pull requests, issues, and other GitHub concepts to
the terminal next to where you are already working with `git` and your code.

![screenshot](https://user-images.githubusercontent.com/98482/73286699-9f922180-41bd-11ea-87c9-60a2d31fd0ac.png)

## Availability

While in beta, GitHub CLI is available for repos hosted on GitHub.com only. It does not currently support repositories hosted on GitHub Enterprise Server or other hosting providers. We are planning support for GitHub Enterprise Server after GitHub CLI is out of beta (likely toward the end of 2020), and we want to ensure that the API endpoints we use are more widely available for GHES versions that most GitHub customers are on. 

## We need your feedback

GitHub CLI is currently early in its development, and we're hoping to get feedback from people using it.

If you've installed and used `gh`, we'd love for you to take a short survey here (no more than five minutes): https://forms.gle/umxd3h31c7aMQFKG7

And if you spot bugs or have features that you'd really like to see in `gh`, please check out the [contributing page][]

## Usage

- `gh pr [status, list, view, checkout, create]`
- `gh issue [status, list, view, create]`
- `gh repo [view, create, clone, fork]`
- `gh help`

## Documentation

Read the [official docs](https://cli.github.com/manual/) for more information.

## Comparison with hub

For many years, [hub][] was the unofficial GitHub CLI tool. `gh` is a new project for us to explore
what an official GitHub CLI tool can look like with a fundamentally different design. While both
tools bring GitHub to the terminal, `hub` behaves as a proxy to `git` and `gh` is a standalone
tool. Check out our [more detailed explanation](/docs/gh-vs-hub.md) to learn more.


<!-- this anchor is linked to from elsewhere, so avoid renaming it -->
## Installation

### macOS

`gh` is available via Homebrew and MacPorts.

#### Homebrew

Install: `brew install github/gh/gh`

Upgrade: `brew upgrade gh`

#### MacPorts

Install: `sudo port install gh`

Upgrade: `sudo port selfupdate && sudo port upgrade gh`

### Windows

`gh` is available via [scoop][], [Chocolatey][], and as downloadable MSI.

#### scoop

Install:

```
scoop bucket add github-gh https://github.com/cli/scoop-gh.git
scoop install gh
```

Upgrade: `scoop update gh`

#### Chocolatey

Install:

```
choco install gh
```

Upgrade:

```
choco upgrade gh
```

#### Signed MSI

MSI installers are available for download on the [releases page][].

### Debian/Ubuntu Linux

Install and upgrade:

1. Download the `.deb` file from the [releases page][]
2. `sudo apt install ./gh_*_linux_amd64.deb` install the downloaded file

### Fedora Linux

Install and upgrade:

1. Download the `.rpm` file from the [releases page][]
2. `sudo dnf install gh_*_linux_amd64.rpm` install the downloaded file

### Centos Linux

Install and upgrade:

1. Download the `.rpm` file from the [releases page][]
2. `sudo yum localinstall gh_*_linux_amd64.rpm` install the downloaded file

### openSUSE/SUSE Linux

Install and upgrade:

1. Download the `.rpm` file from the [releases page][]
2. `sudo zypper in gh_*_linux_amd64.rpm` install the downloaded file

### Arch Linux

Arch Linux users can install from the AUR: https://aur.archlinux.org/packages/github-cli/

```bash
$ yay -S github-cli
```

### Other platforms

Install a prebuilt binary from the [releases page][]

### [Build from source](/docs/source.md)

[docs]: https://cli.github.com/manual
[scoop]: https://scoop.sh
[Chocolatey]: https://chocolatey.org
[releases page]: https://github.com/cli/cli/releases/latest
[hub]: https://github.com/github/hub
[contributing page]: https://github.com/cli/cli/blob/master/.github/CONTRIBUTING.md
