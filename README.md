# GitHub CLI

`gh` is GitHub on the command line, and it's now available in beta. It brings pull requests, issues, and other GitHub concepts to
the terminal next to where you are already working with `git` and your code.

![screenshot of gh pr status](https://user-images.githubusercontent.com/98482/84171218-327e7a80-aa40-11ea-8cd1-5177fc2d0e72.png)

## Availability

While in beta, GitHub CLI is available for repos hosted on GitHub.com only. It currently does not support repositories hosted on GitHub Enterprise Server or other hosting providers. We are planning on adding support for GitHub Enterprise Server after GitHub CLI is out of beta (likely towards the end of 2020), and we want to ensure that the API endpoints we use are more widely available for GHES versions that most GitHub customers are on.

## We want your feedback

We'd love to hear your feedback about `gh`. If you spot bugs or have features that you'd really like to see in `gh`, please check out the [contributing page][].

## Usage

- `gh pr [status, list, view, checkout, create]`
- `gh issue [status, list, view, create]`
- `gh repo [view, create, clone, fork]`
- `gh config [get, set]`
- `gh help`

## Documentation

Read the [official docs][] for more information.

## Comparison with hub

For many years, [hub][] was the unofficial GitHub CLI tool. `gh` is a new project that helps us explore
what an official GitHub CLI tool can look like with a fundamentally different design. While both
tools bring GitHub to the terminal, `hub` behaves as a proxy to `git`, and `gh` is a standalone
tool. Check out our [more detailed explanation][gh-vs-hub] to learn more.


<!-- this anchor is linked to from elsewhere, so avoid renaming it -->
## Installation

### macOS

`gh` is available via Homebrew and MacPorts.

#### Homebrew

Install:

```bash
brew install gh
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

### Fedora, Centos, Red Hat Linux

Install:

```bash
sudo dnf config-manager --add-repo https://cli.github.com/packages/rpm/gh-cli.repo
sudo dnf install gh
```

Upgrade:

```bash
sudo dnf install gh
```

### Other Linux

See [Linux installation docs](/docs/install_linux.md)

### Other platforms

Download packaged binaries from the [releases page][].

### Build from source

See here on how to [build GitHub CLI from source][build from source].

[official docs]: https://cli.github.com/manual
[scoop]: https://scoop.sh
[Chocolatey]: https://chocolatey.org
[releases page]: https://github.com/cli/cli/releases/latest
[hub]: https://github.com/github/hub
[contributing page]: https://github.com/cli/cli/blob/trunk/.github/CONTRIBUTING.md
[gh-vs-hub]: /docs/gh-vs-hub.md
[build from source]: /docs/source.md
