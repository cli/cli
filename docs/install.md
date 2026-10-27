# Install

Linux and macOS only. Windows is not supported.

Use the `go` and `toolchain` versions in [go.mod](../go.mod).

## From a checkout

```sh
make install
```

`prefix` defaults to `/usr/local`. You may need `sudo`. To install elsewhere:

```sh
make install prefix=/path/to/gh
```

Run `gh version` to check the binary on your `PATH`.

Pre-releases built by this fork are described in [releasing.md](releasing.md). They are not the Homebrew `gh` formula and not the apt or yum packages published for `cli/cli`.

## Cross-compile

```sh
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 make clean bin/gh
```

This fork builds and runs on `linux` and `darwin` only. `go tool dist list` includes other `GOOS` values. Those targets are not supported.
