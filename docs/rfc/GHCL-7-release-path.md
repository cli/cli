# GHCL-7: Which command ships this fork

Status: draft

## Summary

`script/release` without `--local` dispatches `deployment.yml` on `cli/cli`. The workflow that publishes this repository is `.github/workflows/prerelease.yml`. Pick one release entry and make the other stop talking to `cli/cli`.

## Motivation

A maintainer who follows the old release command does not ship this tree. With a token that can dispatch `cli/cli`, the command starts upstream's production deployment (`dry_run=false`). The pre-release workflow is what this fork added, and it is the only release workflow in `.github/workflows/`. It also skips two steps the GoReleaser path still performs: `script/licenses` before the build, and macOS signing. `gh licenses` on a pre-release binary therefore prints the placeholder in `internal/licenses/licenses.go`.

## Design

Keep `.github/workflows/prerelease.yml` as the release that publishes this repository.

Change `script/release` so that, without `--local`, it dispatches `prerelease` on the repository that contains the script, not `cli/cli` and not `deployment.yml`. Pass the selected ref through. Do not set `environment=production` on a workflow that has no such input.

Before `go build` in that workflow, run `script/licenses` for the job's `GOOS` and `GOARCH`, the same hook `.goreleaser.yml` already runs, so `report.txt` is embedded.

Leave `script/release --local` as the GoReleaser build for people who want archives or Linux packages on disk. Do not describe it as the published release.

## Alternatives considered

Restore a `deployment.yml` in this repository and point `script/release` at it. That brings back GoReleaser packaging, signing secrets, and the cli.github.com publish steps. This fork already replaced that workflow with the pre-release build. Restoring it reopens Windows-sized release machinery the fork removed.

Delete `script/release` and document only a manual `workflow_dispatch`. That also stops the `cli/cli` dispatch. It throws away the `--local` GoReleaser entry, which still matches `.goreleaser.yml`.

Leave the script as it is and only document the danger. The next rebase will not stop a typed command from targeting `cli/cli`.

## Open questions

Should published macOS binaries be signed and notarized, or is an unsigned cross-compile from Linux acceptable for this fork?

Should the Linux artifact names drop `musl` until the build uses a musl toolchain? The static-ELF check does not establish a musl sysroot.

## Decision

`script/release` without `--local` dispatches `prerelease.yml` on the repository that contains the script, passing only the selected ref, and that workflow runs `script/licenses` before `go build`.
