# Shared helper interfaces

The bundled internal skills use one runtime and evidence format. They are
instructions loaded into the current agent context, not separate worker agents.

## Contents

- [Preflight receipt](#preflight-receipt)
- [Runtime](#runtime)
- [Live requests](#live-requests)
- [Environment and privacy](#environment-and-privacy)
- [Evidence layout](#evidence-layout)
- [Offline evidence rendering](#offline-evidence-rendering)

## Preflight receipt

Build the Go helper using [the offline-first prerequisite flow](prerequisites.md).
`<helper> --skill-root <package-root> preflight check --output <receipt.json>`
performs checks only. It must never install or update a dependency. Optional
`--module-root`, `--node`, and `--font` select caller-approved existing tools.

A ready receipt has this shape:

```json
{
  "schemaVersion": 1,
  "status": "ready",
  "manifestSha256": "<hash of the pinned runtime manifests>",
  "tools": {
    "node": {"path":"/absolute/node","version":"..."},
    "tuistory": {"moduleRoot":"/absolute/node_modules","version":"..."},
    "ffmpeg": {"path":"/absolute/ffmpeg","version":"..."},
    "ffprobe": {"path":"/absolute/ffprobe","version":"..."},
    "font": {"path":"/absolute/font.ttf","family":"..."}
  }
}
```

Missing or incompatible capabilities produce `status: "needs-install"` or
`status: "blocked"` with itemized reasons and proposed remedies.
If setup is needed, the agent proposes commands using the pinned requirements,
asks the caller to approve them, and rechecks after executing the approved setup.
An omitted module-root selection is reported as unselected, not proof that the
library is absent. The helper has no installation commands or install-state
manager. Package-manager success alone is not readiness.

## Runtime

`<helper> --skill-root <package-root> run --contract <case.json> --preflight <receipt.json>`
starts one finite Go owner. Go interprets the contract, enforces action policy,
runs controllers, evaluates assertions, and owns capture and external file IPC.
It launches a thin Node adapter using the receipt's Node/Tuistory installation.
The adapter only calls the native terminal API and exchanges real data over
private stdio. It does not interpret contracts or decide what the case may do.
Preflight drives this same adapter, including verifying that a quiet resize
produces an updated snapshot.
No shared global Tuistory daemon is started.
The current owner requires Unix process-group and file-ownership support;
other platforms are blocked rather than receiving an unverified cleanup promise.

The launcher stays in the foreground until the run closes. For live interaction,
start it as an owned background command attached to the current agent, then
use the client from that agent. Do not detach or daemonize it. Read the launcher's
ready response and use an observation request to confirm that it is responsive:

```json
{"status":"ready","runDir":"/absolute/private/workspace","runId":"..."}
```

The runtime checks the pinned manifest hash, installed Tuistory version, selected
Node identity, and target executable checksum. `command.version` is provenance
supplied by the caller, not permission to run an additional version command.
Both repository and standalone skill installations are supported. Generated
data must remain outside the skill and any enclosing or current repository.

`<helper> client --run-dir <workspace> --file <request.json>` sends one request.
Omit `--file` to read one JSON object from stdin. Private atomic transport files live in
`ipc/requests` and `ipc/responses`; `runtime.json` identifies their owner and
lifecycle. No playback server is started.

The client's optional `--timeout` bounds its response wait, not case timing.
A client timeout does not retry an action: the submitted request might still
complete. Observe before deciding what to do next.

## Live requests

Every request has `op`. Begin with `{"op":"observe"}`. Successful active
observation, wait, and action responses include `ok`, `observation`, `revision`,
and `privacyPending`.
The observation contains the real visible `text`, cell-based `cursor`,
`columns`, `rows`, `alternateScreen`, `exited`, `exitCode`, and capabilities.
Recognizable line menus also expose their `question`, `selected` label,
options, and checkbox states.

| Operation | Additional fields | Requires latest revision and reason |
| --- | --- | --- |
| `observe` | None | No |
| `wait` | `until` condition or `milliseconds`, plus optional `timeoutMs` | No |
| `note` | `chapter`, `text` | No |
| `act` | Canonical `action` | Yes |
| `run_controller` | Bounded `controller` | Yes |
| `finish`, `shutdown` | None | Yes, while active |

For example, replace the illustrative revision with the latest response value:

```json
{
  "op": "act",
  "revision": 3,
  "reason": "The supplied case asks for this key.",
  "action": {"type":"key","key":"enter"}
}
```

Stale observations are rejected, not silently refreshed. Revisions track real
screen, cursor, alternate-buffer, or mouse-capability changes rather than every
irrelevant PTY byte. An error response has `ok:false` and `error.code` plus a
safe diagnostic `error.message`.

An action's success confirms input delivery, not that the target has finished
processing it. Its response may still show the preceding screen. Wait for the
needed condition or observe again before deciding the next interaction.

Pure waits and annotations do not authorize extra input:

```json
{"op":"wait","until":{"screenContains":"Ready"},"timeoutMs":5000}
{"op":"wait","milliseconds":1000,"timeoutMs":1500}
{"op":"note","chapter":"INPUT","text":"Enter the supplied value."}
```

These are three separate requests, not one JSON document. Conditions combine
literal `screenContains`, `questionPrefix`, and `selected` fields with AND.
Regular expressions and generated predicates are not accepted.

Canonical actions and timing are defined in [the run contract](run-contract.md).
Keys are supported names, printable ASCII, or modifier chords such as
`["ctrl","c"]`. Separate keypresses must be separate actions, not a chord array.
Ctrl letters are case-insensitive. Shift letters produce their uppercase
characters, optionally with Alt. Enter, Tab, Backspace, and Escape support
combinations of Ctrl, Alt, and Shift. Other keys support Alt only.
Meta, additional modifiers on Ctrl letters, and other chords whose modifiers
Tuistory ignores are rejected before input.
Semantic selection requires a unique label in a supported arrow-marked line
menu. Checkbox toggles verify their requested boolean state. Unsupported or
ambiguous selectors return an error rather than guessing. Mouse clicks use
zero-based cells and require supported SGR mouse reporting from the target.
Resizing emits a fresh terminal snapshot even if the target prints nothing.

Every live and controller action uses the same fidelity and denial gates.
Denials also cover primitives generated by semantic navigation, newlines, and
ordinary terminal key aliases. An unobservable required selection fails closed.
Equivalent key spellings, letter case under Ctrl or Shift, and modifier order
are normalized for denial matching and physical delivery. Exact-step comparisons
and recorded contract values stay unchanged.
If an exact action fails after delivering part of its input, the run stops as
blocked rather than replaying the already delivered prefix.

Load [the controller reference](controllers.md) only when using a controller.
An uncovered controller state can pause without restarting the terminal.
Strategy changes do not authorize changing supplied steps.

Natural target exit closes the runtime automatically. `finish` or `shutdown`
while the target is live interrupts it and produces a blocked run, not a
successful exit. Once closed, `observe`, `finish`, and `shutdown` return the
saved result without starting another process.
Stopping a run cancels pending terminal operations before waiting for action
completion, so an outstanding acknowledgement cannot prevent cleanup.
Cleanup tracks the actual PTY process group, not just its leader's exit.
The adapter waits for the native TERM/KILL escalation and verifies that the
group is gone before acknowledging closure. Go also retains the group identity
for forced cleanup if the adapter fails or its close operation is cancelled.

## Environment and privacy

Only explicitly approved `environment.pass` names enter the Go owner and cross
the adapter's anonymous in-memory pipe. Their values never belong in contracts,
external client requests, arguments, or
logs. HOME, profiles, configuration, caches, and temporary roots are isolated.
There is no automatic account lookup or operator configuration inheritance.

Tuistory 0.11 fixes `TERM=xterm-truecolor` and `COLORTERM=truecolor`; conflicting
reviewed values are rejected. Protocol replies describe real cell geometry,
cursor, and configured colors, not invented pixel-window dimensions.

Declared secrets are checked in source documents, input, raw output, and visible
states, including split output and invisible OSC data. Operator-specific paths
outside declared locations are rejected. Rejected text is not saved and later
presented as successfully redacted output.

A possible credential prefix may temporarily defer evidence and return
`privacyPending:true` with only the last safe observation. The top-level revision
still describes the active UI. Do not infer an exploratory action from withheld
text. Prefer a bounded wait or already-authorized canonical input through the
same gate. Contracts and controller sources cannot contain actual secrets,
including secrets split across literal steps.

These checks and the authorization record are not an OS sandbox. The selected
program still runs with the operator's OS privileges.

## Evidence layout

All generated data lives under the contract's workspace:

```text
contract.json
runtime.json
capture/
  contract.json
  states.jsonl
  events.jsonl
  raw.jsonl
  controllers/
result.json
artifacts/
```

Each state record contains `t` in monotonic seconds, `revision`,
`alternateScreen`, and `data` from the real Tuistory terminal. Each event records
`t`, its `type`, and the execution `mode` (`ai`, `controller`, or `host`).
Go decodes shared terminal types for observations and rendering while preserving
the original `data` JSON, including fields the current reader does not know.
Input events retain the requested action and its actual preceding observation.
Annotations use `type: "note"` with `chapter` and `text` fields.
The `ai` execution mode identifies the live-client interface; it does not attest
that a request came from a model rather than a scripted client.

`result.json` records `caseId`, `command`, `contractSha256`, `durationSeconds`,
`exitCode`, `exitSignal`, `captureStatus`, `caseStatus`, and expectation results.
It also records `stepsCompleted`, `expectedSteps`, `actionsCompleted`,
`stopReason`, `interrupted`, `captureError`, `actionError`, `cleanupError`,
`cleanupDurationSeconds`, and `finalObservation`. `originalContract` points to
the exact-byte copy at `capture/contract.json`. Cleanup time is excluded from
capture duration.

Runtime `startedAt` and `finishedAt` metadata lets session assembly check actual
execution chronology. Preserve it without edits. Nonchronological, overlapping,
or reused captures cannot be assembled as a valid sequential session. Legacy
captures lacking these timestamps remain readable only with an explicit
`execution order unverified` warning, not a claim of verified chronology.

Capture is `complete` only after a normal, fully captured exit and successful
cleanup. Actual nonzero exit codes remain unchanged. Interruptions, signals,
limits, partial input, or capture failures produce `failed` capture status and
retain the evidence. The runtime bounds captured and pending data to 64 MiB;
exhausting it is an explicit capture failure.
The adapter also bounds its unread event backlog to 64 events. Overflow reports
an incomplete capture while keeping response delivery available for cleanup;
it does not silently discard output and report success.
The Node bridge separately limits pending protocol output to 64 MiB, reserving
4 KiB for failure and cleanup messages. A stalled consumer cannot grow that
queue indefinitely. Overflow stops capture and the owned target without waiting
for the consumer, and exits unsuccessfully. Already queued output can drain;
after a capture failure, a final flush has a 3.5-second limit before the adapter
exits with an error. The byte budget applies to pending data, not lifetime output.

Case statuses are `passed`, `failed`, `blocked`, and `observed`. Missing required evidence,
unsupported expectations, and unfinished exact steps cannot pass.

The evidence helper reads this layout without contacting the target service.
It writes media and JSON evidence under `artifacts/`, with optional HTML for
interactive review.
Cleanup closes only the runtime's owned process groups and transport. It cannot
contain a program that deliberately daemonizes into a separate session.
Preserve the contract and evidence; never delete the user's repository or
unrelated files.

## Offline evidence rendering

For the normal final deliverable, including a single case, use
`evidence --session <manifest.json>` with [a recording session](recording-sessions.md):

```sh
<helper> --skill-root <package-root> evidence \
  --session <manifest.json> --preflight <receipt.json>
```

Each case covers one behavior, with its preparation, one exercise invocation,
and immediate validation captured before the next case. Assembly preserves
actual order, not a rearrangement of previously batched phases. The annotated
session includes a task-specific overview and continuous whole-session progress.
Every readable overview page lasts 10 seconds; long lists gain pages rather
than losing cases. Fidelity is derived from exercise contracts as `Exact run`,
`Exploration`, or `Mixed run`, and recorded with overview timing in rendered
session metadata. No new session input fields are needed.

The low-level form
`<helper> --skill-root <package-root> evidence --run-dir <workspace> --preflight <receipt.json>`
remains available for per-invocation clips. It is not the normal path to a final
session video with overview and whole-session progress.
Go rasterizes the recorded terminal cells and captions from the explicit
OpenType fonts, places terminal and overview content inside a framed 48-pixel
safe area so common playback controls do not obscure the first row, then streams
frames to FFmpeg. It writes a new private rendering directory rather than
replacing prior evidence. Only media is delivered by default; raw capture, JSON
reports, and inspection files remain internal evidence.
Add `--html` to also generate a self-contained HTML report. Without it, neither
standalone nor session renders create HTML, including per-chapter reports.
Use `--inspection all` when the claim
requires every source state; the default extracts representative samples.

To make an additional presentation from the same capture, supply `--format gif`
or `--format mp4` (repeat for both), and/or `--timing realtime|condensed`.
When timing is unspecified in the contract, presentation defaults to condensed.
Explicit contract timing is preserved unless `--timing` overrides it. No separate
approval is required for condensed presentation:

```sh
<helper> --skill-root <package-root> evidence \
  --session <manifest.json> --preflight <receipt.json> \
  --format mp4 --timing condensed
```

Keep each run's manifest `verification` reference for manual expectations, or
the `--verification` argument when rendering a per-invocation clip.
The selected formats must have passed preflight. These flags affect only the
new rendering, not the command, assertions, original contract, or raw capture.
The report records a separate `presentation` object and retains the original
contract hash. Without overrides, output follows the recorded contract, using
condensed timing only when that field is omitted. `--timing-authorization` may
optionally record the caller's request for a condensed companion.

Rendering can also select `--font FILE` and repeated `--font-fallback FILE`
options. Without `--font`, rendering uses the font selected by the ready
preflight receipt. Supplying `--font-fallback` adds the explicit fallback chain
in the supplied order. Relative paths resolve from the helper's working
directory. Unusable fonts fail rendering explicitly, not the recorded case's
outcome. Legacy contracts containing font paths remain readable, but new
execution contracts do not record presentation font paths.

These options work with both `--run-dir` and `--session`, including the session
overview. They are saved in `presentation.fontPath` and
`presentation.fontFallbacks`; successful renderings also identify the actual
font files and SHA-256 hashes in `rendering.font` and `rendering.fontFallbacks`.
To recover an existing capture with a missing glyph, select a compatible
fallback and rerun only evidence rendering:

```sh
<helper> --skill-root <package-root> evidence \
  --session <manifest.json> --preflight <receipt.json> \
  --font-fallback /absolute/fonts/symbols.ttf
```

Neither the immutable execution contract nor any captured output is rewritten.
No subject command is rerun, and no new execution preflight receipt is needed
solely to select rendering fonts.

Annotated session output labels preparation in purple and running in blue.
Validation has visible result text, green for passed expectations and red for
failed ones; unknown, blocked, and observed results must not look passed.
Show the immutable recorded executable and arguments above the captured
terminal output, not as invented keystrokes or a second shell execution.
The lower panel explains the run in plain language rather than `Command: ...`.
A thin progress line spans overview pages, reading holds, and chapter boundaries
without restarting. Explicit unannotated real-time output keeps all of these
additions off. See [session presentation](recording-sessions.md#present-the-session).

The helper uses the same assertion evaluator as the live Go owner, applying it
to the actual exit code and final
recorded terminal of a complete invocation. Interrupted runs, failed captures,
and incomplete exact cases remain blocked; partial final-screen text or a
termination signal does not establish a product failure. Unsupported expectation
types are explicitly blocked. `--verification <file.json>` can supplement only manual
expectations with this shape:

```json
{
  "contractSha256": "<exact contract byte hash>",
  "results": [
    {
      "id": "<manual expectation id>",
      "status": "passed",
      "reason": "What the independent observation established.",
      "evidence": ["verification/readback.json"]
    }
  ]
}
```

Evidence references must identify existing files inside the private workspace.
This records the agent's verification; it is not cryptographic attestation.
It cannot override an automatic failure or make unfinished exact steps pass.
The helper preserves case outcome when rendering fails and reports the latter
separately. Optional HTML embeds media and requires no playback server.

Only requested and preflight-probed formats are rendered. GIF-only output does
not require the MP4 encoder for a standalone per-invocation clip; session
assembly does require MP4 chapter intermediates, even for a GIF-only final
deliverable. `rendering.mediaDetails` records each media file's
hash, dimensions, decoded frame count, actual duration, and reported frame rate.
GIF timing has centisecond granularity; the timeline's duration is not an exact
timing assertion for that format.
Final output is retained even when it arrives between frame ticks. Real-time
reports separate `captureDurationSeconds` from encoded timeline duration and
disclose any frame-quantization tail as `endHoldSeconds`.

`inspection/manifest.json` binds media hashes to image hashes. Identical rendered
pixels, including dimensions, are stored once under `inspection/images/`.
This compares actual pixels, not just terminal text; cursor, color, selection,
and caption changes remain distinct.

The manifest's `rendered` entries map inspected video frame ranges to an image,
capture time, source-state index, revision, and annotation time. Repeated
appearances retain separate entries even when they share an image.
`sourceStates` maps every captured state to its image, time, revision, and
alternate-screen flag when `--inspection all` is selected. The original
capture journals and video frames are unchanged. The existing `sampled` mode
still provides sampled coverage; deduplication does not turn it into an
exhaustive review.

`encodedSamples` identifies first, middle, and final frames decoded from each
actual output. Inspect those images too rather than judging codec quality from
source images alone.

Final session inspection honors the same `sampled`/`all` choice and records it
as `mode`. `decodedFrames[format]` maps selected final-video frame numbers to
decoded images. See [session inspection](recording-sessions.md#assemble-the-video)
for coverage; the existing chapter reports retain source-state details.

Rendering rejects frames above 64 megapixels, font sizes above 512 points, and
timelines above one million frames rather than allocating unbounded resources.
This leaves the original case result and capture intact.
Visual review remains pending in the original generated report. After actually
inspecting the images, preserve a separate `visual-review.json` beside it:

```json
{
  "contractSha256": "<exact contract byte hash>",
  "manifestSha256": "<inspection manifest byte hash>",
  "status": "passed",
  "coverage": "First, final, and all supplied validation transitions.",
  "images": ["inspection/images/image-000000.png"],
  "findings": []
}
```

Use `passed`, `failed`, or `blocked` to describe that visual review only. List
actual inspected paths; do not claim every state from representative samples.
Keep the report's at-rendering status and point to this completed receipt at
handoff. The receipt records the reviewing agent's observations, not automated
proof of privacy or rendering fidelity.
