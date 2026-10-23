---
name: vhs-demo
description: Use when creating a terminal GIF, recording a CLI demonstration, authoring a VHS tape, or producing visual before/after evidence for reviewers.
compatibility: Requires VHS 0.11.0+, ffmpeg, ffprobe, ttyd, and a VHS-supported shell.
---

# VHS Demo

Produce a short, repeatable GIF that shows real commands and unedited output from
caller-selected executables. The GIF is evidence of one observed flow, not proof
of general correctness and not a replacement for tests.

## Evidence contract

A finished demo must make these claims defensible:

- VHS rendered a saved tape; no fallback recorder or `vhs publish` was used.
- Every visible command ran against an identified executable.
- Hidden setup created state but did not create, paste, rewrite, or overlay the
  output attributed to the product.
- Configuration, credentials, hosts, and data were isolated and synthetic by
  default.
- The native-resolution animation was inspected, including any transient state.

The caller decides where the artifact is ultimately used. Return local paths and
evidence; do not require an upload, PR edit, or repository change.

## Workflow

### 1. Define the claim

Record the intended audience or destination, the behavior to evidence, exact
visible commands, required initial state, and any size constraint. Storyboard
the visible beats and their approximate duration before writing the tape.

Prefer one coherent claim per GIF. Use a synthetic supporting service or fixture
only when the real product cannot safely or deterministically reach the required
state; disclose and remove it afterward. The visible command must still run the
selected real executable. Never substitute a fake or wrapper that generates
product output. Split success and error flows when combining them would obscure
either result.

For before/after evidence, identify the controlled variable. Keep the shell,
terminal geometry, commands, typing speed, synthetic data, and equivalent
application state fixed.

### 2. Prove the toolchain and executables

Run the platform preflight from the repository root:

```sh
.github/skills/vhs-demo/scripts/preflight.sh
```

```powershell
& .\.github\skills\vhs-demo\scripts\preflight.ps1
```

If VHS, ffmpeg, ffprobe, or ttyd is missing or cannot execute, or VHS is older
than 0.11.0, name the failure and stop. Do not install tools or substitute
asciinema, screen recording, hosted VHS, or fabricated terminal text.

The caller chooses every executable, including separate baseline and changed
binaries. Before recording each one, capture its absolute path, version, source
or revision, and preferably SHA-256. Build separate binaries from separate
checkouts; never overwrite one binary or switch the active checkout between
takes.

Invoke absolute paths in visible commands. If natural spelling such as `gh` is
part of the presentation, create an isolated command-name mapping to that exact
path and put it ahead of the tape's `PATH`. Do not trust aliases or the ordinary
`PATH`.

### 3. Isolate the take

Use the platform's secure temporary-directory API to create one workspace per
GIF. Keep the tape, GIF, fixtures, config, command mappings, and inspection
frames there, never in the repository.

Use synthetic names, repositories, hosts, and content. Remap home and config
roots into the workspace, then clear inherited credential variables and
quarantine every config source the selected product can read. For `gh`, point
`GH_CONFIG_DIR` at an empty private directory and clear `GH_TOKEN`,
`GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, `GH_HOST`, and
`GH_REPO`. If the behavior inherently needs a service, use only an approved
synthetic endpoint and account; stop if safe credentials or data are unavailable.

Select a shell accepted by `Set Shell` in the installed VHS version and use only
that shell's syntax in `Type` commands. An executable being installed does not
mean VHS supports it. VHS 0.11 shell definitions suppress startup profiles and
set a generic prompt; confirm both with a probe before trusting prompt waits or
`PATH`.

### 4. Author and render the tape

Adapt this structure, replacing every placeholder:

```text
Output "<ABSOLUTE_TEMP_PATH>/demo.gif"
Require <SHELL_COMMAND>
Set Shell <SHELL_COMMAND>
Set Width <WIDTH_FOR_CONTENT>
Set Height <HEIGHT_FOR_CONTENT>
Set FontSize 22
Set FontFamily "<VERIFIED_MONOSPACE_FONT>"
Set TypingSpeed 35ms
Set CursorBlink false
Set WaitTimeout 120s

Env GH_CONFIG_DIR "<ABSOLUTE_TEMP_PATH>/config"
Env HOME "<ABSOLUTE_TEMP_PATH>/home"
Env XDG_CONFIG_HOME "<ABSOLUTE_TEMP_PATH>/config"
Env GH_TOKEN ""
Env GITHUB_TOKEN ""
Env GH_ENTERPRISE_TOKEN ""
Env GITHUB_ENTERPRISE_TOKEN ""
Env GH_HOST ""
Env GH_REPO ""
Env NO_COLOR "1"

Hide
Type "<SHELL-SPECIFIC SETUP AND CLEAR>"
Enter
Wait+Line /<DETERMINISTIC PROMPT>/
Show

Type "<EXPLICIT EXECUTABLE> <VISIBLE ARGUMENTS>"
Enter
Wait+Screen /<MEANINGFUL EXPECTED OUTPUT>/
Sleep 2s
```

Use hidden commands only for fixtures, isolated config, approved synthetic
services, deterministic prompts, directory changes, and clearing the screen.
Type every visible command and let it run. Use `Wait+Screen` for observable
results and condition-based waits for prompts; use `Sleep` only for pacing.
Let waits capture spinners and other motion rather than sleeping past them.

Verify the selected font is installed and resolves as monospaced using the
platform font registry or a native-resolution ruler probe. Render a sample
frame, then size width, height, and font for the content. Increase geometry
instead of shrinking or clipping text.

Run `vhs <tape>`. A parse or directive failure after preflight is an
incompatibility: report it rather than deleting a safeguard.

### 5. Inspect the evidence

From the repository root, pass the matching helper an inspection directory
outside the repository:

```sh
.github/skills/vhs-demo/scripts/inspect.sh <demo.gif> <inspection-dir>
```

```powershell
& .\.github\skills\vhs-demo\scripts\inspect.ps1 <demo.gif> <inspection-dir>
```

The helper rejects repository paths, verifies GIF metadata, and extracts
native-resolution first, final, one-frame-per-second, and every-frame sequences.

Inspect the images and the tape together. Confirm executable selection, visible
commands, unedited output, intentional wrapping, readable monospaced text,
clean first and final frames, useful pacing, and absence of secrets, private
paths, real hosts, identities, notifications, and hidden setup. Inspect every
frame when checking privacy or any spinner, crash, exit, relaunch, or other
transient claim. Fix unsafe input at its source and rerender. Never redact the
GIF after capture.

### 6. Return evidence

Report the absolute tape and GIF paths; dimensions, duration, frame count when
available, and size; visible commands; each executable's path, version,
revision/source, and hash when recorded; whether sampled or all frames were
inspected; the intended use; and any limitation on what the demo proves.

## Stop shortcuts

| Shortcut | Required response |
| --- | --- |
| "Use my existing login/config" | Isolate config and use synthetic credentials or stop. |
| "Both clips say `gh`, so selection is obvious" | Map the name to each proven binary explicitly. |
| "Any recorder is fine if VHS is missing" | Stop and report the failed prerequisite. |
| "Put the GIF in the repo for convenience" | Keep all generated media in a temporary workspace. |
| "The final frame proves the spinner" | Inspect the transient frame sequence. |
