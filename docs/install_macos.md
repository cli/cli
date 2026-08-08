# Installing gh on macOS

## Recommended _(Official)_

### Homebrew

[Homebrew](https://brew.sh/) is a free and open-source software package management system that simplifies the installation of software on Apple's operating system, macOS, as well as Linux.

The [GitHub CLI formulae](https://formulae.brew.sh/formula/gh) is supported by the GitHub CLI maintainers with help from our friends at Homebrew with updates powered by [homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core/blob/main/Formula/g/gh.rb).

To install:

```shell
brew install gh
```

To upgrade:

```shell
brew upgrade gh
```

### Precompiled binaries

[GitHub CLI releases](https://github.com/cli/cli/releases/latest) contain precompiled binaries for `amd64` and `arm64` architectures along with a universal installer.

> [!NOTE]
> As of May 29th, Mac OS installer `.pkg` are unsigned with efforts prioritized in [`cli/cli#9139`](https://github.com/cli/cli/issues/9139) to support signing them.

## Community _(Unofficial)_

> [!IMPORTANT]
> The GitHub CLI team does not maintain the following packages or repositories. We are unable to provide support for these installation methods or any guarantees of stability, security, or availability for these installation methods.

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

### Flox

[Flox](https://flox.dev/) is a virtual environment and package manager all in one. With Flox you create environments that layer and replace dependencies just where it matters, making them portable across the full software lifecycle.

Flox relies upon the [GitHub CLI package](https://github.com/NixOS/nixpkgs/blob/master/pkgs/by-name/gh/gh/) supported by the [NixOS community](https://nixos.org/)

To install:

```shell
flox install gh
```

To upgrade:

```shell
flox upgrade toplevel
```

### MacPorts

[MacPorts](https://www.macports.org/) is an open-source community initiative to design an easy-to-use system for compiling, installing, and upgrading either command-line, X11 or Aqua based open-source software on the Mac operating system.

The [GitHub CLI port](https://ports.macports.org/port/gh/) is supported by the MacPorts community with updates powered by [macports/macports-ports](https://github.com/macports/macports-ports/blob/master/devel/gh/Portfile).

To install:

```shell
sudo port install gh
```

To upgrade:

```shell
sudo port selfupdate && sudo port upgrade gh
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
