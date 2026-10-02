---
name: cli-exercise-run
description: Internal shared execution and recording flow for cli-exercise. Uses one Go owner and a thin Tuistory adapter for exact or exploratory cases.
user-invocable: false
license: MIT
---

# Shared execution

Read [the runtime interface](../../references/runtime-interface.md). Read the
[controller format](../../references/controllers.md) only when authoring or
running a controller. Require a ready preflight receipt and a complete approved
contract before launching the subject.
Use the [session manifest and shared-state convention](../../references/recording-sessions.md)
even for a single case. Each case covers one behavior: execute its `setup` runs,
one `exercise` invocation, then its `validation` runs immediately, before the
next case. Approved `cleanup` follows validation. Multiple setup or validation
runs are allowed; a second exercise belongs to a separate behavior case, unless
doing so would change an exact caller case, which requires clarification first.
Honor explicitly narrower coverage without inventing missing phases.

Each invocation uses this same run/client path with its own capture directory.
Run titles explain what preparation establishes or validation checks. Carry
approved shared state forward, not a reused capture. Wait for each run to finish
and close before starting the next; do not batch phases or overlap case runs.
Keep actual execution order in the manifest. The assembler is not a way to
repair history by rearranging clips.

## Start an owned, isolated run

Use the receipt's tools and the contract's executable, arguments, and workspace.
Verify the executable checksum immediately before launch. Never fall back to a
different `gh`, shell, or executable found on the ordinary PATH.

Start the Go helper's `run` command with the skill root, contract, and preflight receipt. Keep the process
attached to the current task. Do not use a global Tuistory daemon or start
unrelated background agents. A bounded temporary host may remain alive while
the current agent inspects and acts; it must be shut down at the end.

The Go runtime isolates configuration and passes only explicitly approved
environment values. Never type, print, or persist real credentials. These
controls do not make the host an OS sandbox for untrusted programs.
Node only supplies the mechanical Tuistory boundary. It must not replace Go's
contract checks, controller decisions, capture policy, or verdicts.

## Observe, then act

Use the client to read the actual terminal state, then request an allowed action
with that observation's revision and a concise reason.

Do not confuse a visible choice with the currently selected choice, or focus
with a checked checkbox. A stale-observation rejection requires a fresh read,
not an automatic replay of the same action.

Use condition-based waits for readiness and explicit timing for pacing when
the case permits it. Record the actual typing, prompt redraws, spinners, editor
transitions, and failures. Never assemble a fake terminal from selected stills.

Use semantic selector helpers only where their interpretation is supported.
For other permitted UIs, inspect and use the caller's required raw keys or
coordinates. Report unsupported interactions explicitly.

## Controllers and handoffs

Controllers are bounded declarative data. Their actions pass through the same
exact-step and effect checks as live AI actions. Go interprets their rules; they
cannot execute arbitrary code or grant themselves broader authority.

An uncovered prompt may pause a controller while retaining the terminal.
In exact mode, the next action still must match the supplied case. In explore
mode, the AI can choose the next permitted action after inspecting the state.

Annotate meaningful steps and decisions while they happen. Keep annotations
outside the product output and preserve their source timestamps.

## Finish on every exit path

Capture the actual exit status and final state. Ask the runtime to finish and
shut down. If the subject cannot finish normally, stop only the owned process
tree and report interruption distinctly from the expected outcome.

Retain the runtime's `startedAt` and `finishedAt` metadata for chronology checks.
Never fill in or rewrite timestamps to make a capture fit the intended order.
Legacy captures without them require an explicit `execution order unverified`
warning, not a claim of verified chronology.

Do not leave a host or recorder running while writing the report. Preserve
partial evidence and use [the evidence module](../cli-exercise-evidence/SKILL.md)
even after a failure.
