---
name: cli-exercise-exact
description: Internal exact-case flow for cli-exercise. Preserves supplied commands, inputs, ordering, assertions, and output constraints.
user-invocable: false
license: MIT
---

# Record exact cases

Require the entry skill's preserved request, validated contracts, ready
preflight receipt, and approved effect scope. If any is missing, return to the
entry skill instead of filling gaps with assumptions.

## Normalize without changing meaning

Keep the caller's original case text. Turn supplied commands and interactions
into the canonical actions in [the contract](../../references/run-contract.md).
Keep test ordering and case IDs stable.

Each recording case covers one behavior, with at most one exercise invocation.
Give it a very short behavior/outcome title, not an inventory of commands or
flags. A supplied case may include multiple preparation or validation commands.
If its exact steps conflict with this structure, ask before splitting, reordering,
omitting, or adding anything. Explicit narrower coverage takes precedence over
the default preparation/exercise/validation coverage.

If the caller specifies literal keys, preserve those keys. If they specify
an option by name, a mechanical selector can locate it. Ask before inventing
values or steps that materially affect the case. Do not add negative tests,
extra metadata, new retries, or exploratory branches just because they seem useful.

Keep before/after conditions controlled: equivalent initial state, commands,
geometry, data, and timing, with separately identified executables.

## Execute faithfully

Use [the shared run module](../cli-exercise-run/SKILL.md). The exact-action gate
must remain active for both live AI actions and generated controllers.

Execute each case's preparation, exercise, and validation in that order before
starting the next case; approved cleanup can follow its validation. Do not batch
phases across cases or later rearrange captured runs to suggest a different
execution order. Preserve and clarify conflicting caller-supplied ordering.

Observe the real screen before actions. Condition-based waits can synchronize
unspecified timing, but cannot replace or defeat a stated timing requirement.
Never match an expected output merely because it appears in the typed command.

An unexpected prompt is not permission to reinterpret the test. Use only
mechanical actions consistent with the supplied case. Otherwise pause, preserve
evidence, and report what needs clarification.

If an authorized write is part of the case, execute it as specified or report
blocked. Do not replace it with Cancel and claim that the case passed. Follow
all applicable environment/tool policies.

## Judge the specified assertions

Check every expectation against actual observations, with references to the
appropriate state, output, file, or approved external readback.
Do not rewrite the expectation to match the observed result.

Use `failed` for contradicted expectations and `blocked` for requirements that
could not be exercised or verified. An execution with no correctness assertion
is `observed`, not `passed`. A nonzero exit can pass an expected-error case;
exit zero cannot excuse a contradicted assertion.

Finish with [evidence and cleanup](../cli-exercise-evidence/SKILL.md).
