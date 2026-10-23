---
name: cli-exercise
description: Exercises real command-line programs from explicit cases or a goal, with faithful terminal recordings and observed results. Use when asked to record CLI test cases, demonstrate command behavior, validate an interactive workflow, or explore a command against a goal; not for ordinary unit-test execution or skill authoring.
license: MIT
compatibility: Requires Go 1.27+ to build the helper, Node with Tuistory, ffmpeg/ffprobe, and an installed monospaced font. Builds start offline; ask before downloading missing dependencies.
---

# CLI Exercise

Exercise the caller's selected executable and return local evidence. Accept
precise cases, a broad goal, or a mixture. Preserve explicit requirements while
letting the agent choose useful investigations where the caller left room.

By default, produce one chaptered session video, even for one case. Each case
covers one behavior and has a very short natural-language behavior/outcome title.
Actually execute and record that case's preparation, one exercise invocation,
then validation before starting the next case. Approved cleanup can follow
validation. Never batch all setups or validations or reorder historical clips
to imply that execution order. Exact supplied steps and narrower coverage remain
authoritative: ask about conflicts rather than silently adding, splitting, or
reordering a caller's case.

Deliver media only by default. Infer clear output intent: a video exercising a
PR calls for an MP4, while a request for an interactive review report calls for
media plus HTML. Ask only when the requested deliverable is ambiguous. Generate
HTML with `--html` only when wanted; do not create or open it for video-only
requests. Keep raw captures and JSON results as internal evidence.

The internal `SKILL.md` files below are bundled instruction modules. Read the
needed module from this package and follow it in the current agent context.
They do not require new agents, sessions, or separately installed sibling skills.

## Load only what is needed

This entry file is the routing overview. Load the selected flow, not every
internal module. Execute the Go helper as a tool; read its source only when
diagnosing a problem or reviewing the implementation.

Go owns prerequisite checks, execution policy, controllers, capture,
verification, and rendering. Node is only a mechanical adapter to Tuistory's PTY
API. The skill does not require another scripting-language environment.

| Need | Resource to read |
|---|---|
| Check readiness or request installation | [Preflight flow](internal/cli-exercise-preflight/SKILL.md) |
| Preserve supplied cases | [Exact-case flow](internal/cli-exercise-exact/SKILL.md) |
| Decide how to pursue a goal | [Exploration flow](internal/cli-exercise-explore/SKILL.md) |
| Start or drive the terminal | [Run flow](internal/cli-exercise-run/SKILL.md) |
| Verify, render, or clean up | [Evidence flow](internal/cli-exercise-evidence/SKILL.md) |
| Normalize the caller's cases | [Run-contract reference](references/run-contract.md) |
| Call the bundled helpers | [Runtime interface](references/runtime-interface.md) |
| Coordinate cases and assemble one video | [Recording sessions](references/recording-sessions.md) |
| Author a deterministic controller | [Controller reference](references/controllers.md) |
| Resolve missing prerequisites | [Prerequisite reference](references/prerequisites.md) |
| Inspect representative routing behavior | [Input examples](references/examples.md) |
| Maintain or evaluate the package | [Maintenance reference](references/maintenance.md) |
| Run an authoring evaluation | [Agent-behavior rubrics](tests/skill-evaluations.json) |

These direct links keep resources one hop from the entry point. For an exact
request, do not load the exploration flow unless the caller also supplied an
exploratory case. Do not load controller details for a live-only run.

## 1. Preserve the request and choose the fidelity policy

Classify each case, not just the entire request:

- **Exact:** the caller supplied cases or required steps. Preserve commands,
  arguments, input values, order, assertions, and timing/output constraints.
  Do not add exploratory steps or silently substitute an equivalent-looking
  operation for a specified key sequence.
- **Explore:** the caller supplied a goal or expected outcome. Choose checks
  and interactions within the agreed context and permitted effects. Separate
  supplied expectations from your hypotheses.

Input fidelity is independent of execution strategy. Either policy can use a
bounded controller, live observation/action calls, or both. Switching strategy
never grants permission to change an exact case.

Resolve only missing details that affect correctness or authority:

- The executable or build to exercise, with its actual path and identity.
- Cases or goal, supplied context, expected outcomes, and initial state.
- Permitted files, hosts, accounts, repositories, and other effects.
- Desired media format and constraints. Honor an explicit format; use GIF for
  an unspecified terminal animation and MP4 when the caller requests video.

Default to condensed presentation, with static pauses shortened. Honor explicit
real-time requests and retain the original capture timestamps. Keep timing
adjustments in the recording metadata, not repeated on-screen labels.

A request such as "upload an attachment" is not permission to choose an
arbitrary account or publish to the current repository. Ask for the missing
target/authorization before performing a write. Prefer synthetic data and
isolated configuration.

Command discovery is a separate case property. A case may provide the command
and test its use, or explicitly require finding it. Do not silently turn a
known-command case into a discoverability test.

Preserve the non-secret original request alongside normalized cases. Never copy
credentials into the contract or recording; use approved secret references or
report blocked if the requested literal recording would disclose them. Use
[the run contract](references/run-contract.md); unsupported or ambiguous
requirements must be clarified or marked blocked, not discarded.

## 2. Establish readiness and authorization

Read [the preflight module](internal/cli-exercise-preflight/SKILL.md).
It checks dependencies. If setup is needed, propose commands using the pinned
requirements, obtain caller approval, and recheck afterward.

Installation approval and permission to modify test resources are independent.
Record the executable path, version/revision, checksum, accepted case contract,
and approved effect scope before starting the subject.

Use a secure private workspace outside the repository. Do not put recordings,
credentials, runtime caches, or operator configuration in this skill directory.
No secret values belong in the contract or a tool request.

## 3. Load the appropriate internal flow

- Supplied cases: [record exact cases](internal/cli-exercise-exact/SKILL.md).
- Broad goals: [explore a goal](internal/cli-exercise-explore/SKILL.md).

Both use [the shared run module](internal/cli-exercise-run/SKILL.md).
It provides real terminal observation, input, controller execution, recording,
and bounded process ownership. Read only the internal modules needed.
Use [a recording session](references/recording-sessions.md) to keep case and phase
order, carry approved state between related commands, and assemble their actual
recordings with `evidence --session`, not the per-invocation `--run-dir` path.
Keep validation commands in that recording. The session overview uses the
task-specific title, derived fidelity, and case results, with 10 seconds per
readable page. Progress spans the entire final video.

## 4. Verify, render, and clean up

Always finish through [the evidence module](internal/cli-exercise-evidence/SKILL.md),
including when the case is blocked or the subject fails.

Return the case outcome, meaningful observed behavior, requested media paths,
executable identity, inspection coverage, and limitations. Keep case failure,
capture failure, and render failure distinct. A video is evidence of an observed
flow, not proof of general correctness or a replacement for tests.

Include a report link only when the caller requested the report. Do not leave
a host, terminal, recorder, or playback server running. Publishing the evidence
is a separate, explicit action.
