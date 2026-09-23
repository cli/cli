# Prerequisites and installation consent

The helper is a self-contained Go program. Go owns the font/image pipeline as
well as prerequisite checks and execution policy. Node is required only for
Tuistory's native terminal API.

## Contents

- [Build the Go helper without silent downloads](#build-the-go-helper-without-silent-downloads)
- [Check existing capabilities](#check-existing-capabilities)
- [Receipts and compact output](#receipts)
- [Agent-led setup](#agent-led-setup)
- [Offline regression tests](#offline-regression-tests)

## Build the Go helper without silent downloads

Locate an installed Go 1.27+ toolchain. On the initial version check, disable
automatic toolchain downloads with `GOTOOLCHAIN=local` and ignore saved Go
configuration with `GOENV=off`. If the selected toolchain is too old, use an
explicitly selected existing compatible toolchain or ask before installing one.
Do not let the repository's toolchain directive silently download it.

Build from the bundled [Go module](../tool/go.mod) into the private task
workspace. The following is POSIX shell syntax; use equivalent per-process
environment settings on other platforms:

```sh
GOENV=off GOFLAGS= GOTOOLCHAIN=local GOWORK=off GOPROXY=off GOSUMDB=off \
  <selected-go> -C <package-root>/tool build \
  -mod=readonly -trimpath -buildvcs=false -o <workspace>/cli-exercise .
```

This may use already cached locked modules and the Go build cache, but it cannot
download modules or change the dependency manifests. Keep the resulting binary
outside the skill and repository. Rebuild once per task unless a caller supplied
a binary whose current source identity has already been established.

If the build fails because a locked module is absent, do not retry online yet.
Read `tool/go.mod` and `tool/go.sum`, then present the exact pinned modules,
public Go proxy/checksum sources, selected module-cache destination, and build
command. Ask the caller to approve that specific dependency fetch and build.
After approval, the same read-only build may use the reviewed
`GOPROXY=https://proxy.golang.org` and `GOSUMDB=sum.golang.org`, with
`GOPRIVATE`, `GONOPROXY`, and `GONOSUMDB` explicitly cleared and no inherited
proxy/index credentials. Do not update module versions or run `go mod tidy` as
part of normal skill execution. Declined approval, unavailable Go, missing
checksums, or compilation failures are blocked, not a successful preflight.

The font/image and terminal-control dependencies are compiled into the helper;
no font-rendering service or separate package environment is needed afterward. Its Node
dependency installation remains a separate, explicitly approved operation.

## Check existing capabilities

```sh
<helper> --skill-root <package-root> preflight check --output /absolute/workspace/preflight.json
```

Use `--module-root` to identify the installed Tuistory `node_modules` directory.
If it is omitted, the checker reports that no installation was selected rather
than assuming the package is absent. Locate a suitable existing installation
before proposing a new one.

`check` never installs, updates, downloads packages, or starts a shared daemon.
It creates a private, short-lived probe workspace, runs bounded local fixtures,
cleans them up, and writes the requested receipt. It does not use the operator's
home, account variables, or application configuration for the
fixtures. Existing executable shims that require a personal profile may fail;
select their underlying executable explicitly instead.

Optional selections:

| Flag | Meaning |
| --- | --- |
| `--module-root PATH` | Existing **node_modules directory**, not the Tuistory package directory |
| `--node PATH` | Existing Node executable; defaults to PATH discovery |
| `--ffmpeg PATH` | Existing ffmpeg executable |
| `--ffprobe PATH` | Existing ffprobe executable |
| `--font PATH` | Optionally validate an existing monospaced TTF, OTF, or TTC file planned for rendering |
| `--format gif` / `--format mp4` | Repeat for requested formats; both are checked by default |
| `--probe-timeout SECONDS` | Per-process bound, 5-120 seconds; default 20 |

For example, to check explicitly supplied dependencies without modifying them:

```sh
<helper> --skill-root <package-root> preflight check \
  --module-root /absolute/dependencies/node_modules \
  --font /absolute/fonts/monospace.ttf \
  --output /absolute/workspace/preflight.json
```

Readiness requires more than version output:

- Tuistory **0.11.0** must work through the production terminal adapter. Go drives
  that same bridge to run the helper's private `probe-fixture` command. The probe
  checks raw and visible output, quiet resize snapshots, text and key delivery,
  actual exit status, and owned cleanup. The bridge validates the required
  Tuistory methods, including `clickAt`, and the pinned PTY handle used for
  process-group cleanup before reporting readiness. Both the bridge and target
  use isolated environments. No separate Node probe or global
  daemon is used. The Go fixture has a four-second deadline, and cleanup has a
  separate five-second bound.
- Go font checks load and draw with the actual TTF/OTF/TTC file and verify equal
  advances for all printable ASCII characters.
  This is not a promise of coverage for every Unicode glyph.
- MP4 checks encode two real frames with `libx264` and `yuv420p`, exercise looped
  PNG input for overview pages, and check the `drawbox`, `color`, and `overlay`
  filters used for whole-video progress. GIF checks use per-frame `palettegen`,
  `paletteuse`, and the GIF encoder so rendering need not
  buffer a whole live session. ffprobe must decode the outputs
  and confirm their dimensions, codec, and frame count. Request only formats
  needed by the case, and do not use a receipt to claim unprobed formats.
  Normal session delivery needs MP4 chapter intermediates, including a single
  case or a GIF-only final session. Probe MP4 and any requested GIF format;
  only standalone per-invocation GIF clips can omit MP4 capability.
  A native sample also exercises `select`, passthrough frame timing, and RGB
  decoding for inspection of the actual encoded media.

Known font locations are checked on macOS, Linux, and Windows. `fc-match` is not
required. Missing fonts or unsupported native PTY libraries are reported, not
replaced with another recorder or guessed compatible configuration. Node 26.7,
Go 1.27, and ffmpeg 9 are development examples, not a blanket platform
compatibility claim. The actual capability probes decide readiness.

## Receipts

With `--output`, stdout contains status, the saved report path, problem
descriptions, and the next permitted step.
Load the saved JSON when tool paths, installation details, or full diagnostics
are needed. Do not load helper source during normal execution; inspect it only
when debugging the helper itself.

A ready receipt has `schemaVersion: 1`, `status: "ready"`, `manifestSha256`,
and these `tools` entries:

```json
{
  "node": {"path": "/absolute/node", "version": "observed version"},
  "tuistory": {"moduleRoot": "/absolute/node_modules", "version": "0.11.0"},
  "ffmpeg": {"path": "/absolute/ffmpeg", "version": "observed version"},
  "ffprobe": {"path": "/absolute/ffprobe", "version": "observed version"}
}
```

Checks and itemized failures accompany the receipt. Missing pinned packages can
produce `needs-install`; missing system tools, unusable native capabilities, or
unverified safety conditions produce `blocked`. Failed receipts contain only
the tool details that could be observed. Argument failures may not
have tool observations at all. Exit codes are 0 for ready, 1 for needs-install,
and 2 for blocked or invalid input.

`manifestSha256` hashes `scripts/package.json` then `scripts/package-lock.json`.
For each, the hash input is its base
UTF-8 filename, a zero byte, its byte length as an eight-byte big-endian integer,
and its exact contents. Executable hashes are additional provenance, not an OS
trust or permission mechanism.

## Agent-led setup

Use the appropriate package manager for setup, keeping it separate from
capability checking:

1. Use the selected platform and pinned manifests to propose exact commands.
   State the versions, trusted sources, destination, and any global changes.
2. Ask the caller to approve those commands before downloading or installing.
3. Execute the approved setup, then rerun `preflight check` with the actual paths.

For Tuistory, use the bundled `scripts/package.json` and `package-lock.json`.
A typical setup copies them into a new private dependency directory outside the
skill and repository and runs `npm ci` against the public registry. Keep optional
native packages enabled and lifecycle scripts disabled. Do not replace an
existing installation or change the pinned versions implicitly.

Go, Node, FFmpeg, fonts, and platform-specific repairs may need different setup
commands. The agent determines those from the environment rather than a custom
installation framework. Preserve any partial-install facts and do not claim
readiness until the real capability checks pass.

Terminal execution requires Unix process-group cleanup and ownership checks.
Platforms where these are not implemented remain blocked even if their
dependencies are installed; installing more packages does not resolve that
limitation.

## Offline regression tests

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  <selected-go> -C .github/skills/cli-exercise/tool test -mod=readonly ./internal/preflight
```

The constructor/CLI and run-behavior tables use test-owned files and process
fakes. They never contact GitHub, package registries, an operator's account, or
the real dependency cache. Native local validation is separate and must use
explicitly selected existing dependencies when no installation was approved.
