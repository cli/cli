---
name: writing-evals
description: Use when creating or revising GitHub CLI agent evals, especially live fixtures, skill-off/skill-on comparisons, polling or event-routing behavior, Vally definitions, deterministic graders, or slow and flaky model loops.
---

# Writing Agent Evals

## Status

This is tentative guidance from the first event-driven `gh` eval suite. Preserve
counterexamples and revise it when repeated trials disagree.

## Core principle

Distinguish desired behavior from nearby success. Completion through polling is
not evidence that event streaming worked.

Define the treatment effect first:

```text
skill off: normal graded failure attributable to missing guidance
skill on: task, route, no-polling, and semantic checks all pass
```

Infrastructure, credential, actor, and cleanup errors are not valid baseline
failures.

## Working sequence

1. **Specify observable behavior.** Keep command names and anti-polling policy
   out of the normal user prompt.
2. **Separate contracts.** Test the candidate command, harness/grading, and agent
   adoption independently.
3. **Build deterministic evidence.** Record versioned treatment provenance,
   actual argv/output, actor actions, exit status, and final state.
4. **Tighten the loop.** Unit tests → offline replay → no-model live smoke →
   shortened model trial → checked-in-budget off/on comparison.
5. **Convert failures into regressions.** Do not debug deterministic logic with
   repeated five-minute model runs.

Prefer Vally definitions plus executor/grader plugins over a bespoke runner.
Keep the shell wrapper thin; TypeScript can own fixtures and graders without
owning the runner.

## Grading contract

| Grader | Independent question |
| --- | --- |
| Task | Did the externally observable requested outcome happen? |
| Route | Did a dedicated event command observe each exact scripted change? |
| No polling | Did repeated unchanged status reads substitute for waiting? |
| Semantic | Does the full trajectory honestly satisfy the request? |

Semantic review cannot rescue deterministic failure. Require exact event output
or post-listener snapshot recovery, not merely temporal overlap.

Treat one initial read and one event-triggered read per external-change epoch as
potentially legitimate. Repeated successful reads of the same projection
without an intervening change are polling.

## Non-negotiable safeguards

- Use a separate actor identity and trigger it from explicit readiness or
  observed progress, never arbitrary control delay.
- Keep the baseline skill unchanged; make treatment baseline-plus-delta.
- Require subscriber, actor, fixture, and model credentials explicitly. Never
  default the evaluated identity to personal `gh` authentication.
- Journal resources first; clean up every exit path with the owning identity.
- Keep design probes and product-hint interventions in separate variants.
- Version and deeply validate replay evidence; normalize old versions explicitly.

## Common mistakes

| Mistake | Better direction |
| --- | --- |
| LLM decides command facts | Parse actual argv, output, timing, and state |
| Latency stands in for streaming | Require route evidence and detect polling |
| Stream and snapshot start together | Wait for attachment, then snapshot |

Read [the design notes](references/design-notes.md) for details and use
[the eval runbook](../../../evals/running-evals.md) for repository commands.
