# GitHub CLI

`gh` is GitHub on the command line. It brings pull requests, issues, and other GitHub concepts to the terminal next to where you are already working with `git` and your code.

![screenshot of gh pr status](https://user-images.githubusercontent.com/98482/84171218-327e7a80-aa40-11ea-8cd1-5177fc2d0e72.png)

GitHub CLI is available for repositories hosted on GitHub.com and GitHub Enterprise Server 2.20+, and to install on macOS, Windows, and Linux.

## Documentation

For [installation options see below](#installation), for usage instructions [see the manual][manual].

## Contributing

If anything feels off, or if you feel that some functionality is missing, please check out the [contributing page][contributing]. There you will find instructions for sharing your feedback, building the tool locally, and submitting pull requests to the project.

If you are a hubber and are interested in shipping new commands for the CLI, check out our [doc on internal contributions][intake-doc].

<!-- this anchor is linked to from elsewhere, so avoid renaming it -->
## Installation

### macOS

`gh` is available via [Homebrew][], [MacPorts][], [Conda][], [Spack][], and as a downloadable binary from the [releases page][].

#### Homebrew

| Install:          | Upgrade:          |
| ----------------- | ----------------- |
| `brew install gh` | `brew upgrade gh` |

#### MacPorts

| Install:               | Upgrade:                                       |
| ---------------------- | ---------------------------------------------- |
| `sudo port install gh` | `sudo port selfupdate && sudo port upgrade gh` |

#### Conda

| Install:                                 | Upgrade:                                |
|------------------------------------------|-----------------------------------------|
| `conda install gh --channel conda-forge` | `conda update gh --channel conda-forge` |

Additional Conda installation options available on the [gh-feedstock page](https://github.com/conda-forge/gh-feedstock#installing-gh).

#### Spack

| Install:           | Upgrade:                                 |
| ------------------ | ---------------------------------------- |
| `spack install gh` | `spack uninstall gh && spack install gh` |

### Linux & BSD

`gh` is available via:
- [our Debian and RPM repositories](./docs/install_linux.md);
- community-maintained repositories in various Linux distros;
- OS-agnostic package managers such as [Homebrew](#homebrew), [Conda](#conda), and [Spack](#spack); and
- our [releases page][] as precompiled binaries.

For more information, see [Linux & BSD installation](./docs/install_linux.md).

### Windows

`gh` is available via [WinGet][], [scoop][], [Chocolatey][], [Conda](#conda), and as downloadable MSI.

#### WinGet

| Install:            | Upgrade:            |
| ------------------- | --------------------|
| `winget install --id GitHub.cli` | `winget upgrade --id GitHub.cli` |

> **Note**  
> The Windows installer modifies your PATH. When using Windows Terminal, you will need to **open a new window** for the changes to take affect. (Simply opening a new tab will _not_ be sufficient.)

#### scoop

| Install:           | Upgrade:           |
| ------------------ | ------------------ |
| `scoop install gh` | `scoop update gh`  |

#### Chocolatey

| Install:           | Upgrade:           |
| ------------------ | ------------------ |
| `choco install gh` | `choco upgrade gh` |

#### Signed MSI

MSI installers are available for download on the [releases page][].

### Codespaces

To add GitHub CLI to your codespace, add the following to your [devcontainer file](https://docs.github.com/en/codespaces/setting-up-your-project-for-codespaces/adding-features-to-a-devcontainer-file):

```json
"features": {
  "ghcr.io/devcontainers/features/github-cli:1": {}
}
```

### GitHub Actions

GitHub CLI comes pre-installed in all [GitHub-Hosted Runners](https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners).

### Other platforms

Download packaged binaries from the [releases page][].

### Build from source

See here on how to [build GitHub CLI from source][build from source].

## Comparison with hub

For many years, [hub][] was the unofficial GitHub CLI tool. `gh` is a new project that helps us explore
what an official GitHub CLI tool can look like with a fundamentally different design. While both
tools bring GitHub to the terminal, `hub` behaves as a proxy to `git`, and `gh` is a standalone
tool. Check out our [more detailed explanation][gh-vs-hub] to learn more.

[manual]: https://cli.github.com/manual/
[Homebrew]: https://brew.sh
[MacPorts]: https://www.macports.org
[winget]: https://github.com/microsoft/winget-cli
[scoop]: https://scoop.sh
[Chocolatey]: https://chocolatey.org
[Conda]: https://docs.conda.io/en/latest/
[Spack]: https://spack.io
[releases page]: https://github.com/cli/cli/releases/latest
[hub]: https://github.com/github/hub
[contributing]: ./.github/CONTRIBUTING.md
[gh-vs-hub]: ./docs/gh-vs-hub.md
[build from source]: ./docs/source.md
[intake-doc]: ./docs/working-with-us.md
