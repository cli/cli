---
name: cli-exercise-evidence
description: Internal evidence, rendering, and cleanup flow for cli-exercise. Verifies expectations and returns local artifacts without publishing by default.
user-invocable: false
license: MIT
---

# Evidence and cleanup

Read the immutable contract, runtime result, input/controller trace, and recorded
states. Keep the caller's case outcome separate from capture and rendering status.
For the default complete recording, follow [recording sessions](../../references/recording-sessions.md)
and assemble all requested cases and phases with `evidence --session`, even for
one case. `--run-dir` remains useful for a per-invocation clip, not the normal
session deliverable.

## Verify the requested claim

Check every expectation against actual evidence. Built-in terminal assertions
do not prove separate stdout/stderr routing or remote resource state.
Complete manual expectations with approved independent observations, or mark
them blocked. Never infer success solely from exit zero or a produced video.
Record any case-validation commands as session chapters rather than leaving
their execution outside the video. Execute them immediately after that case's
exercise, before the next case, not as a final batch when assembling evidence.
Honor exact supplied steps and explicit narrower coverage; ask about conflicts
rather than adding validation commands the caller excluded.

For writes, verify the object actually written rather than an unrelated or
pre-existing object. Preserve failures and partial-effect facts. Do not repeat
a potentially successful write just because readback failed.

For exact cases, confirm that the supplied steps, values, order, and assertions
were preserved. For exploration, distinguish supplied outcomes from inferred
checks and record meaningful changes of approach.

Judge pass/fail against expectations, not exit-code positivity. An expected
nonzero exit can pass; an exit-zero assertion failure cannot. Keep `observed`,
`blocked`, and unknown results visibly distinct from `passed`.

## Render without fabricating output

Use the Go helper's `evidence` command with the skill root, session manifest,
and ready receipt. Honor the requested GIF/MP4 formats and size constraints.
Condensed presentation is the default when timing is unspecified. Honor explicit real-time requests.
Retain source timing and record shortened pauses and reading holds in the
rendering metadata. Do not add repeated timing labels to the video.
Select an existing compatible primary font, pass it with `--font FILE`, and let
Go rasterize it; FFmpeg encodes the resulting frames.
Do not add an interpreter, browser, or font service as an unapproved fallback.

For another output from an existing capture, use `--format` and/or `--timing`
on the evidence command. Condensed output does not require a separate approval.
Each invocation writes a new rendering and records its presentation options
separately. Do not rewrite
the execution contract or rerun a command merely to change its presentation.

Every rendering requires `--font FILE`. If it reports a missing glyph, repeat
`--font-fallback FILE` to supply an ordered fallback chain. These options apply
to session chapters and the overview too.
Select existing compatible fonts, then rerun only `evidence`; the new report
records the rendering choices and font hashes without changing the capture.

The default deliverable is the requested media, without HTML. Add `--html` only
for an interactive report request. This applies to both standalone and session
renders, including per-chapter reports. Raw capture, JSON results, and inspection
data remain available regardless of that choice.

For an annotated session, require:

- An opening summary with the task-specific manifest title, fidelity derived
  from exercise contracts (`Exact run`, `Exploration`, or `Mixed run`), and brief
  case-title/result rows. Give every readable page 10 seconds. Paginate long
  lists; never drop cases or shrink them into unreadable text.
- Visible phase labels: purple `Preparation`, blue `Running`, and `Validation`
  in green only for passed checks or red for failed checks. Blocked, observed,
  and unknown validation results need explicit non-passing labels and styling.
- A thin progress line spanning the whole final video, including overview pages,
  reading holds, and case boundaries, without restarting for each clip.
- The actual recorded invocation at the top of the terminal area, followed by
  captured output. Present the immutable executable and arguments, not a
  fabricated keystroke transcript or an extra shell execution to print them.
- A separate lower panel explaining the behavior or check in plain language,
  not `Command: ...`. Use run titles and recorded notes; keep product output
  intact and unobscured.

An explicit unannotated real-time request retains no-overlay behavior: do not
add an overview, phase labels, command header, progress line, or explanation
panel. Do not add condensed reading holds.

The terminal picture must come from recorded real states. Never cover, replace,
paste, or retrospectively redact product output. Do not reorder captures to
make historical execution appear sequential. Nonchronological, overlapping,
or reused captures must not be presented as a valid sequential session.
Legacy captures missing runtime timestamps remain readable only with an
explicit `execution order unverified` warning.

If private input caused unsafe output, stop and fix the input at its source.
A new take must respect the original case and any remaining authorization for
effects. Do not repeat remote writes without resolving their existing outcome.

## Inspect before handoff

Inspect native-resolution first/final frames and relevant transient states.
Check readable monospaced text, wrapping, editor/cursor behavior, captions,
timing, and privacy. Inspect every overview page, its 10-second duration, case
coverage, fidelity label, outcome styling, invocation display, and progress at
holds and chapter boundaries. Confirm no overlay appears in an unannotated cut.
Inspect every distinct visual state for a privacy or
transient-state claim. The helper saves identical pixel images once and retains
their frame, timestamp, source-state, and revision mappings in the manifest.
Use those mappings to locate repeated appearances rather than expecting a
separate PNG for each frame.
State actual coverage. Decoding every frame is not the same as visually
reviewing every frame.
Inspect the `encodedSamples` from each requested format as well as the
composited/source images. The latter alone cannot reveal encoding artifacts.

Record executable identity, contract/controller hashes, dimensions, duration,
frame counts when available, and any limitation on what the evidence proves.
Use each format's observed media metadata, including GIF's centisecond timing
granularity, rather than treating the planned frame rate as exact media timing.

The generated report records visual review as pending at rendering time. After
inspection, write a separate `visual-review.json` beside that report, bound to
its contract hash and `inspection.manifestSha256`. List the actual image paths
reviewed, coverage, findings, and the review outcome. Do not rewrite raw capture
or relabel sampled coverage as exhaustive. Link this review receipt at handoff.

## Close and return

Confirm that owned runtime, recorder, and transport processes are stopped.
Do not stop other terminal sessions or globally shut down another user's daemon.
Keep contracts, results, traces, and final artifacts in the private workspace.
Clean up only known temporary data owned by the run.

Return the requested local media paths and concise case outcome. Include HTML
only when requested; it must work without a standing playback server. Publishing
evidence or modifying a PR is separate and requires the caller's request and
appropriate approval.
