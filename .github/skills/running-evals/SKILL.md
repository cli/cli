---
name: running-evals
description: Use when asked to run, compare, debug, re-run, or interpret the GitHub CLI event evals, including skill-off/skill-on trials, review-loop, design probes, replay, smoke testing, cleanup, or saved artifacts.
---

# Running Event Evals

Execute the requested eval end to end. Read
[the eval runbook](../../../evals/running-evals.md) for current commands and
credential roles; do not reconstruct flags from memory. Read
[credential setup](references/credentials.md) before obtaining secrets and
[execution modes](references/execution-modes.md) before deciding who runs.

## Safety contract

- Use the working-tree `./bin/gh`, never an installed `gh`.
- Require an explicitly authorized sandbox organization and live-resource run.
- Never infer `GH_EVAL_TOKEN` from personal `gh` authentication.
- A user may explicitly authorize their current `gh` token for
  `COPILOT_GITHUB_TOKEN`; that authorization does not extend to fixture access.
- Never print tokens.
- Do not fetch, push, commit, publish, or change Git remotes; running evals does
  not require repository network operations.

## Execution sequence

1. **Choose the executor.** Follow the execution-mode decision unless ownership
   is already explicit.
2. **Identify the case.** Honor the requested scenario/variant. If the user asks
   for the strongest default comparison, use `review-loop` with skill `both`.
3. **Prepare.** Build `gh`, install/build the eval package when needed, and
   verify the runner, skill directory, token presence, and target organization.
   Follow the credential setup reference for anything missing. Do
   not broaden scopes or create credentials silently.
4. **Use the cheapest useful gate.**
   - Grader/harness change: offline replay first.
   - Command/API change: replay, then no-model live smoke.
   - Adoption-only rerun: proceed to a shortened model trial.
5. **Development trial.** Use `--agent-budget 2m` or `3m`; run only the affected
   scenario and treatment.
6. **Final measurement.** Remove the budget override. Run the exact requested
   off/on comparison with workers serialized and no retries. Use the definition's
   default single trial unless the caller explicitly requests repetitions.
7. **Interpret.** Separate task, route, polling, semantic, infrastructure, and
   cleanup outcomes. A skill-off result is useful only when it is a normal
   graded failure. Skill-on should pass all four graders.
8. **Verify cleanup.** Confirm the runner reports cleanup complete and inspect
   unfinished journals. Retry cleanup only with the original owning identities.

## Case routing

| Request | Run |
| --- | --- |
| Core treatment effect | `--skill both --gh-skill ./skills/gh` |
| Skill as committed at a revision | `--skill on --gh-skill-ref REV:skills/gh` |
| Capability discovery | `--skill off --design-probe SCENARIO` |
| Grader regression | `evals/replay ARTIFACT` |
| Command/API protocol | `evals/smoke --live --gh ./bin/gh` |
| Subscription limit handling | `listen-at-limit` with `--skill both`, on a subscriber holding no subscriptions |

## Reporting

Lead with the comparison result. Include:

- scenario, variant, model, and budget;
- each grader outcome;
- whether failure was behavioral or infrastructural;
- polling and dedicated-wait evidence;
- cleanup status;
- artifact directories.

Do not claim a treatment effect from one anomalous infrastructure run. Preserve
negative results such as a correctly delivered hint that the model ignored.
