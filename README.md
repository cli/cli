# gh

Linux and macOS fork of [GitHub CLI](https://github.com/cli/cli). This tree has no telemetry, and no extension, attestation, Codespaces, or skill commands. Windows is not supported. The removals are listed in [AGENTS.md](AGENTS.md).

## Install

From a checkout of this repository:

```sh
make install
```

The Go toolchain is declared in [go.mod](go.mod). `make install` uses prefix `/usr/local` unless you set `prefix`. Details are in [docs/install.md](docs/install.md).

Binaries published by this fork are pre-releases from the `prerelease` workflow. See [docs/releasing.md](docs/releasing.md). Packages from cli.github.com and the Homebrew `gh` formula are upstream, not this fork.

## Use

`gh help` prints command help.

License: [MIT](LICENSE).
