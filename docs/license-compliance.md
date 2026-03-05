# License Compliance

GitHub CLI complies with the software licenses of its dependencies. This document explains how license compliance is maintained.

## Overview

Third-party license information is embedded into the `gh` binary at build time using [`google/go-licenses`](https://github.com/google/go-licenses). Each release binary contains the correct license listing for its target platform (GOOS/GOARCH), since the set of dependencies can vary by platform.

## Viewing License Information

Users can view the third-party license information for their installed binary:

```shell
gh licenses
```

This opens a pager displaying all Go dependencies and their licenses, with links to the source code of each dependency.

## How It Works

1. The `script/licenses` script accepts a GOOS and GOARCH and generates a license report using `go-licenses report`
2. The report is written to `internal/licenses/embed/third-party-licenses.md`
3. This file is embedded into the binary via `go:embed` in `internal/licenses/licenses.go`
4. Goreleaser pre-build hooks call `script/licenses` with the correct platform before each build

## Local Development

During local development (`go build`), the embedded file contains a placeholder message. To generate real license information for your current platform:

```shell
make licenses
```

This runs `go-licenses report` for your host GOOS/GOARCH and writes the output to the embed path.
