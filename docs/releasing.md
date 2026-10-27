# Release this fork

Published builds come from [`.github/workflows/prerelease.yml`](../.github/workflows/prerelease.yml). This repository does not run `cli/cli`'s deployment workflow, and it does not publish cli.github.com, Homebrew, or Linux package repositories.

## Dispatch the pre-release workflow

The workflow has no `push` trigger and no inputs. `script/release` without `--local` dispatches `prerelease.yml` on the repository that contains the script. It passes the selected ref and does not pass `environment` or `tag_name`. The ref is `trunk` unless `--branch` sets it. `--current` pushes, then dispatches the upstream branch name. A positional tag name is accepted and is not sent.

```sh
script/release [<tag-name>]
script/release --branch <ref>
```

It builds four binaries on `ubuntu-latest` with `CGO_ENABLED=0` and `go build -trimpath`:

- `linux/amd64` as `gh-x86_64-unknown-linux-musl`
- `linux/arm64` as `gh-aarch64-unknown-linux-musl`
- `darwin/amd64` as `gh-x86_64-apple-darwin`
- `darwin/arm64` as `gh-aarch64-apple-darwin`

The Linux file names include `musl`. The workflow does not use a musl cross-compiler. It is a cgo-disabled Go build. After the build, the workflow checks that each Linux ELF has no dynamic interpreter and no `NEEDED` entry, and that there is one x86_64 ELF and one aarch64 ELF.

Darwin binaries are cross-compiled on Linux. The workflow does not sign or notarize them.

Before `go build`, the workflow runs `bash ./script/licenses "$GOOS" "$GOARCH"` for that matrix entry, the same hook `.goreleaser.yml` uses, so `report.txt` is embedded.

The workflow zips each binary and publishes a GitHub pre-release on this repository. The tag is `release-<full git sha>`, and the release targets that commit.

## Local build

`script/release --local` does not publish. It runs GoReleaser from `.goreleaser.yml` and can write Linux and macOS archives plus Linux deb and rpm packages under `dist/`. GoReleaser's pre-build hook runs `script/licenses`. Install GoReleaser, then run:

```sh
script/release --local
```

That local build is not the pre-release above.

`make site`, `make site-docs`, and `make site-bump` clone `github/cli.github.com` and update upstream's site. They are not part of this pre-release.
