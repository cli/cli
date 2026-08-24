# Installing gh on Windows

## Recommended _(Official)_

### WinGet

[WinGet](https://learn.microsoft.com/en-us/windows/package-manager/winget/) is a command line tool enabling users to discover, install, upgrade, remove and configure applications on Windows 10, Windows 11, and Windows Server 2025 computers. This tool is the client interface to the Windows Package Manager service.

The [GitHub CLI package](https://winget.run/pkg/GitHub/cli) is supported by Microsoft with updates powered by [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs/tree/master/manifests/g/GitHub/cli/).

To install:

```pwsh
winget install --id GitHub.cli --source winget
```

To upgrade:

```pwsh
winget upgrade --id GitHub.cli --source winget
```

> [!NOTE]
> The Windows installer modifies your PATH. When using Windows Terminal, you will need to **open a new window** for the changes to take effect. (Simply opening a new tab will _not_ be sufficient.)

### Precompiled binaries

[GitHub CLI releases](https://github.com/cli/cli/releases/latest) contain precompiled `exe` and `msi` binaries for `386`, `amd64` and `arm64` architectures.

## Community _(Unofficial)_

> [!IMPORTANT]
> The GitHub CLI team does not maintain the following packages or repositories. We are unable to provide support for these installation methods or any guarantees of stability, security, or availability for these installation methods.

### Chocolatey

The [GitHub CLI package](https://community.chocolatey.org/packages/gh) is supported by the Chocolatey community with updates powered by [pauby/ChocoPackages](https://github.com/pauby/ChocoPackages/tree/master/automatic/gh).

To install:

```pwsh
choco install gh
```

To upgrade:

```pwsh
choco upgrade gh
```

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

### Scoop

The [GitHub CLI bucket](https://scoop.sh/#/apps?q=gh) is supported by the Scoop community with updates powered by [ScoopInstaller/Main](https://github.com/ScoopInstaller/Main/blob/master/bucket/gh.json).

To install:

```pwsh
scoop install gh
```

To upgrade:

```pwsh
scoop update gh
```

### Webi

[Webi](https://webinstall.dev/) is a tool that aims to effortlessly install developer tools with easy-to-remember URLs from official builds quickly, without sudo or Admin, without a package manager, and without changing system file permissions.

The [GitHub CLI package](https://webinstall.dev/gh/) is supported by the Webi community with updates powered by [webinstall/webi-installers](https://github.com/webinstall/webi-installers/tree/main/gh).

To install:

```shell
curl -sS https://webi.sh/gh | sh
```

To upgrade:

```shell
webi gh@stable
```
