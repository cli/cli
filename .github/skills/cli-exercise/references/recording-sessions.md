# Recording sessions

The default deliverable is one media-only chaptered session video, even for a
single case. Each case covers one behavior, not an entire command family.
Actually execute its preparation, one exercise invocation, and validation in
sequence before starting the next case. Explicit narrower coverage and exact
supplied steps take precedence; clarify conflicts rather than silently changing
the caller's cases.

## Contents

- [Coordinate the recording](#coordinate-the-recording)
- [Carry state between commands](#carry-state-between-commands)
- [Session manifest](#session-manifest)
- [Preserve execution chronology](#preserve-execution-chronology)
- [Assemble the video](#assemble-the-video)
- [Present the session](#present-the-session)
- [Outcomes and evidence](#outcomes-and-evidence)

## Coordinate the recording

The agent coordinates cases using the existing run/client tools. The session
renderer does not decide what commands to execute or repeat a completed run.

1. Preserve the caller's cases, order, expected outcomes, and scope. Where the
   caller leaves room to plan, separate unrelated behaviors into distinct cases.
   Do not silently split, reorder, or add input to an exact supplied case.
2. Identify the preparation, one exercise invocation, and validation needed for
   the first behavior. Multiple preparation or validation commands are fine.
   Omit phases excluded by the caller; do not invent commands to fill a template.
3. Execute and record that case's preparation, then exercise, then validation
   immediately. Record each invocation through the Go helper with its own
   immutable contract and capture directory. Use live control or controllers.
4. Execute approved explicit cleanup after validation. Only then start the next
   case and repeat the sequence. Never batch all setups or all validations.
5. Add each run to the manifest in its actual execution order. Assemble one
   video after the cases finish. Never rearrange historical clips to make a
   different execution order appear to have happened.

Record readbacks through the same path, rather than substituting a success
caption for unrecorded validation. Wait for each run to finish and close before
launching the next; a recording session is not parallel case execution.

Contract creation, tool compilation, and bookkeeping do not need to appear as
case setup. Operations that establish the actual case state do.
Built-in exit-code and screen assertions are computed checks, not extra command
invocations. Additional readbacks that support the case should be recorded.

The standard manifest phases are `setup`, `exercise`, `validation`, and `cleanup`,
in that order when present. A case has at most one `exercise`; its interactive
actions still belong to that one invocation. Do not disguise a second behavior
as setup or validation to avoid making a separate case. If exact caller steps
conflict with the sequence or case boundary, ask instead of silently normalizing
away the conflict.

Use a task-specific session title and very short natural-language case titles,
such as `gh issue create without arguments prompts`. Avoid implementation or
flag inventories. A run title explains what its preparation establishes, what
its exercise does, or what its validation checks.

## Carry state between commands

Each invocation retains its own `workspace` for transport and capture. Related
commands may name the same explicit `stateDirectory` in their run contracts:

```json
{
  "workspace": "/absolute/session/runs/config-setup",
  "stateDirectory": "/absolute/session/state/config",
  "command": {
    "executable": "/absolute/gh",
    "args": ["config", "set", "prompt", "enabled"],
    "sha256": "<identified executable hash>",
    "version": "<observed version>",
    "cwd": "work"
  }
}
```

This is a fragment of the full [run contract](run-contract.md). With shared
state, `command.cwd` is relative to `stateDirectory`; the isolated home,
configuration, cache, and temporary directories also live there. Setup changes
therefore remain available to exercise and validation without copying operator
configuration or replacing previous evidence.

Shared state must be outside the repository and skill, private to its owner,
and separate from capture directories. Use distinct state directories for
independent cases. Share across cases only when their intended dependency
requires it. Omitting `stateDirectory` keeps the existing per-run isolation.

## Session manifest

Write the manifest outside the repository. Paths may be absolute; relative
`runDirectory` paths resolve against the session workspace. A run's optional
`verification` path resolves against that run's capture directory.

```json
{
  "schemaVersion": 1,
  "id": "local-settings",
  "title": "GitHub CLI local settings",
  "source": "Record saving a prompt setting, storing an alias, and expanding an alias, including preparation and validation for each behavior.",
  "workspace": "/absolute/private/session",
  "cases": [
    {
      "id": "config",
      "title": "gh config set disables prompts",
      "runs": [
        {"id":"baseline","phase":"setup","title":"Establish enabled prompts","runDirectory":"runs/config-setup"},
        {"id":"change","phase":"exercise","title":"Disable prompts","runDirectory":"runs/config-change"},
        {"id":"readback","phase":"validation","title":"Confirm prompts are disabled","runDirectory":"runs/config-readback"}
      ]
    },
    {
      "id": "alias-stored",
      "title": "gh alias set stores an alias",
      "runs": [
        {"id":"alias-baseline","phase":"setup","title":"Establish that the example alias is absent","runDirectory":"runs/alias-baseline"},
        {"id":"define","phase":"exercise","title":"Save the example alias","runDirectory":"runs/alias-define"},
        {"id":"inspect","phase":"validation","title":"Confirm the stored alias definition","runDirectory":"runs/alias-readback"}
      ]
    },
    {
      "id": "alias-expanded",
      "title": "gh alias expands to a command",
      "runs": [
        {"id":"expansion-setup","phase":"setup","title":"Provide the example alias","runDirectory":"runs/expansion-setup"},
        {"id":"invoke","phase":"exercise","title":"Expand the example alias","runDirectory":"runs/expansion-invoke"},
        {"id":"check","phase":"validation","title":"Confirm the expanded command's result","runDirectory":"runs/expansion-readback"}
      ]
    }
  ],
  "output": {"formats":["mp4"],"timing":"condensed"}
}
```

The manifest specifies coverage; it does not invent unrecorded phases or make a
failed command pass. It can describe one run, several cases, or an explicitly
selected subset of previously captured runs.

Use the existing session `title`, case `title`, and run `phase`, `title`, and
`runDirectory` fields for presentation. Do not add fidelity, overview pages, or
summary timing to the session input schema. The renderer derives fidelity and
overview timing and records them in rendered session metadata.

## Preserve execution chronology

The assembler checks runtime `startedAt` and `finishedAt` metadata against
manifest order. Nonchronological, overlapping, or reused captures are rejected,
not silently sorted or treated as fresh executions. Different manifest IDs or
paths do not turn reused capture evidence into independent runs.

Legacy captures without these timestamps remain readable only with an explicit
`execution order unverified` warning. Keep that limitation in the handoff; do
not claim that case adjacency in a video verifies historical execution order.
Never invent timestamps, rewrite original capture or contract bytes, or rerun
effects merely to satisfy presentation checks. A necessary new take requires
appropriate authorization and must be identified as a new execution.

## Assemble the video

```sh
<helper> --skill-root <package-root> evidence \
  --session <session.json> --preflight <receipt.json> --inspection all
```

Session output defaults to MP4. `output.formats` can request GIF, MP4, or both.
Assembly uses MP4 chapter intermediates, so preflight must confirm MP4 and any
other requested format. This does not change standalone GIF rendering.
Use this session path for normal deliverables, even one case or one invocation.
The low-level `evidence --run-dir` path remains useful for an individual clip,
but it does not provide the session overview or whole-session progress.

An explicit session `output.timing` overrides chapter presentation timing;
otherwise each recorded contract's timing is retained, including the condensed
default. `--format` and `--timing` can select another final presentation.
`--font` and repeated `--font-fallback` select rendering fonts for every chapter
and the overview without modifying the recorded contracts. See
[render-time font selection](runtime-interface.md#offline-evidence-rendering).
Use a consistent frame rate across runs. Different dimensions can be padded
without cropping or scaling terminal text.

Condensed chapters shorter than two seconds receive a final-frame reading hold;
`output.minimumChapterSeconds` can change that minimum or set it to zero.
Reading holds stay in the rendering metadata, not a repeated video footer.
Real-time chapters do not receive reading holds.

Media-only is the default deliverable. The MP4 includes chapter metadata and
works independently. Add `--html` when the caller wants an interactive report;
that also creates per-run HTML reports and clickable MP4 chapter navigation.
Without this flag, no HTML is generated. GIF has no native chapter seeking.
No standing playback server is needed.

`--inspection sampled` saves first/middle/last images from the final video.
`--inspection all` also uses the existing chapter frame mappings to cover
terminal and caption changes, overview pages, reading holds, and chapter
boundaries. Images are decoded from the assembled video and deduplicated;
`decodedFrames[format]` maps zero-based frame numbers to their image paths.
This does not change the video or sample every progress-bar tick.

## Present the session

For annotated output, open with a summary using the manifest's task-specific
session title. Derive the fidelity label from the recorded **exercise**
contracts, not setup or validation helper contracts:

| Exercise contract modes | Fidelity label |
| --- | --- |
| Only `exact` | `Exact run` |
| Only `explore` | `Exploration` |
| Both | `Mixed run` |

Show brief case-title/result rows in manifest order. Every case must appear,
including failures, blocked cases, and observations without assertions. Give
each readable summary page **10 seconds**. For long lists, add pages and extend
the intro by 10 seconds per page; never cap it at 10 seconds total, omit cases,
or squeeze text into unreadable rows. Derived fidelity and overview timing
belong in rendered session metadata, not mutated source contracts.

Each chapter has visible phase text as well as color:

| Manifest phase | Visible label and styling |
| --- | --- |
| `setup` | Purple `Preparation` |
| `exercise` | Blue `Running` |
| `validation` | `Validation`, green for passed expectations or red for failed expectations |
| `cleanup` | Explicit `Cleanup` label, not a claim that validation passed |

Keep unknown, blocked, and observed validation results explicitly non-passing;
do not color them green or infer a pass merely because the phase is validation.
Pass/fail is relative to expectations: an expected nonzero exit can pass and an
exit-zero assertion mismatch fails.

Show the actual recorded invocation at the top of the terminal area, followed
by the captured output. Display its immutable executable and arguments without
changing their meaning. This is a presentation of recorded command identity,
not a keystroke transcript or extra shell command to echo the invocation.
Never inject it into the raw terminal capture.

The lower panel is a plain-language explanation of the behavior or check, using
the run title and relevant recorded annotations. It is not `Command: ...` or
an implementation inventory. Keep this panel outside product output.

A thin progress line spans the entire final video, including every summary
page, reading hold, and case boundary. Base progress on the final presentation
timeline, not a per-clip clock; it must not restart at chapter transitions.

Explicit unannotated real-time requests keep their no-overlay behavior. Do not
add a summary, phase labels, invocation header, explanation panel, or progress
line to that cut. Do not shorten its pauses or add condensed reading holds.
Keep original capture and contract bytes intact for every presentation.

## Outcomes and evidence

Keep setup, exercise, validation, and cleanup failures in the declared session. A
contradicted assertion remains failed even when its video renders successfully.
Missing required verification remains blocked. If a requested run cannot be
rendered, assembly reports failure instead of silently omitting that chapter.

For a run with manual expectations, provide its evidence-backed verification
file using the manifest's `verification` field. Evidence references must still
be files inside that run's workspace. A copied validation result should retain
its originating run identity.

Every chapter retains its original contract hash, outcome, capture, and
inspection report. Assembly records the manifest hash and maps chapters onto
the final video timeline. Inspect all overview pages for readable titles and
results, their 10-second timing, the invocation display, phase/status styling,
and continuous progress through holds and encoded chapter boundaries before
handoff. State actual visual-review coverage.
For session-level review, bind the review receipt to the session manifest hash,
final inspection manifest hash, and media hash. List the actual images or frame
indices inspected; per-run reviews remain separate.
