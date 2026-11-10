# Event-driven agent evals

The `events` suite evaluates event-based PR waiting in `gh`. It checks **both completing the task and choosing the dedicated event-based `gh` route**, not just whether the agent eventually gets the answer.

This guide covers what is specific to events. See the [agent evals overview](../../README.md) for the shared harness, installation, credentials, results, isolation, and cleanup, and [Running the event evals](../../running-evals.md) for copyable commands covering the core matrix, design probes, replay, smoke, and cleanup.

## What runs

Each definition is an ordinary Vally `eval.yml`, and the authoring source for its prompts, rubric, graders, scoring, and budgets:

| Definition | User request | Scripted second actor |
| --- | --- | --- |
| [`notify-review`](notify-review/eval.yml) | Let me know when someone reviews my PR. | Submits a review. |
| [`pull-after-merge`](pull-after-merge/eval.yml) | Once this PR is merged, pull the latest main. | Merges the PR. |
| [`pull-after-close`](pull-after-close/eval.yml) | Same request, but the PR closes without merging. | Closes the PR without merging. |
| [`full-delegation`](full-delegation/eval.yml) | Create a PR, respond to reviews, and merge once approved. | Requests a specific fix, verifies the pushed file, then approves. |
| [`review-loop`](review-loop/eval.yml) | Address successive reviews until approved; do not merge. | Requests two distinct fixes on successive heads, then approves the second fix during work. |
| [`listen-at-limit`](listen-at-limit/eval.yml) | Same request as `notify-review`, but the account's subscription limit is already filled, so the agent has to free a slot first. | Submits one comment review. |

Each definition selects the `gh-agent` executor with `config.suite: events` and `config.scenario` naming the live fixture to build. Prompts use `{{PR_URL}}`, which the executor fills in once the suite has created the fixture PR. Every definition has the core stimuli tagged `variant: eval` and `variant: design-probe`, and a `variant: raw-api` stimulus.

Each trial gets a fresh private repository and Git history. `full-delegation` starts with a committed greeting change on a local topic branch, so it tests the review loop rather than substantial feature implementation. The sandbox must not require status checks: check-completion events are outside this first pass.

Normal prompts do not name a waiting command or tell the agent to avoid polling. Command names and flags remain deliberately unspecified. The design-probe stimuli add a hypothetical capability hint without prescribing syntax.

The raw-api stimuli measure how agents use the Event Subscriptions API without `gh event listen`: they append a short description of the HTTP API to the normal prompt, and are meant to run against a `gh` built without the `event` command, such as a trunk build. The command observer only sees `gh` invocations, not `curl` or other HTTP clients, so these stimuli are graded by `event-task` and `semantic-review` only. Compare them with the other variants on those two graders.

## Prerequisites

On top of the [shared prerequisites](../../README.md#prerequisites):

- `GH_EVAL_TOKEN` is the subscriber and should be a **preview-enabled user token**; the suite has only been run with one. The Events API admits a token when its account is enabled for the event subscriptions preview: the user for a user token, or the account the app is installed on for an App installation (`ghs_`) token. Enabling an organization does not admit its members' user tokens.
- `GH_EVAL_ACTOR_TOKEN` is required: a **separate identity** for the second actor, able to review, approve, merge, and close sandbox PRs. A sandbox-scoped App installation token works. The suite refuses to start without it.

If `GH_EVAL_FIXTURE_TOKEN` is supplied, provisioning and repository deletion use that credential, but the subscriber still uses `GH_EVAL_TOKEN`, including for subscription cleanup. This lets a sandbox-scoped App manage repositories without giving the subscriber's user token `delete_repo`; both identities need the appropriate access to the fixtures. Cleanup retries must supply the same roles.

For `review-loop`, the pre-created PR is authored using `GH_EVAL_TOKEN`, even when a fixture App provisions the repository. This keeps the PR author separate from the reviewer App, since GitHub does not allow approving one's own PR.

### Current integration limits

`gh event listen` is in preview. See [the command contract](../../../docs/event-listen-preview.md). Rebuild `bin/gh` before running its evals; a binary from the original baseline does not have the command.

## Fast development loop

Do not use a model trial to debug commands, harness timing, or deterministic graders. Use these gates in order. `evals/replay` and `evals/smoke` are specific to this suite.

### 1. Offline evidence replay

```sh
npm --prefix evals run build
evals/replay ~/.local/state/gh-event-evals/skill-on/<run>/review-loop/review-loop/<model>/0/executor-artifacts
evals/replay evals/testdata/replays/persistent-review-loop.json
```

`evals/replay` runs only `event-task`, `event-route`, and `no-polling`. It uses no network, credentials, model quota, or live resources and normally completes in under a second. Sanitized replay evidence covers persistent streams, numeric repository event URLs, startup-gap recovery, parallel-start failure, and actor delivery regressions.

### 2. Scripted live protocol smoke

```sh
GH_EVAL_HOST=github.com \
GH_EVAL_ORG=gh-evals-sandbox \
GH_EVAL_TOKEN=... \
GH_EVAL_ACTOR_TOKEN=... \
evals/smoke --live --gh ./bin/gh
```

This performs the real two-review loop with the candidate binary but no model: create fixture, attach the stream, wait for the explicit `Listening for events.` readiness diagnostic, snapshot state, submit and observe each review, push both fixes, verify current-head approval, and clean up. It tests the command/API/harness protocol, not skill discoverability. `GH_EVAL_FIXTURE_TOKEN` remains optional. It then creates, views, lists, and deletes a separate subscription with `gh event subscription`, and checks that it is gone.

### 3. Model trial

Only after replay and smoke pass, run the skill trial. During iteration, shorten stuck runs without editing the checked-in definition:

```sh
evals/run --live --gh ./bin/gh --gh-skill ./skills/gh \
  --model MODEL_ID --skill on --agent-budget 2m review-loop
```

Use the checked-in five-minute budget for final measurements and skill-off/on comparisons. More examples:

```sh
# Several definitions, three trials each
evals/run --live --gh ./bin/gh --gh-skill ./skills/gh --model MODEL_ID \
  --runs 3 notify-review pull-after-merge pull-after-close full-delegation

# Design probes: what interface do agents reach for?
evals/run --live --gh ./bin/gh --gh-skill ./skills/gh --model MODEL_ID \
  --design-probe notify-review

# Anything after -- goes to vally eval unchanged, for example --verbose
evals/run --live --gh ./bin/gh --model MODEL_ID --skill off notify-review -- --verbose
```

Each stimulus gives the agent a 5-minute working budget (`constraints.max_agent_duration`) inside a 10-minute trial (`defaults.timeout`) that also covers setup, observation, and cleanup. Trials share one subscriber account and its subscription limit, and the second actor assumes it is the only change in flight, which is why `evals/run` runs them one at a time.

## The skill under test

Skill-on trials offer the shipped `gh` skill, [`skills/gh`](../../../skills/gh/SKILL.md), whose event section covers `gh event listen`, both as a blocking wait for the next event and as a background listener with `--follow`, and briefly the `gh event subscription` management commands. `src/skills.test.ts` checks that it still documents the routes this suite relies on. There is no separate API-based skill variant.

Pass `--gh-skill ./skills/gh` to evaluate the working tree. To compare against a skill without the event section, use `--gh-skill-ref` with a revision from before it was added, such as `trunk:skills/gh` until this change merges. This choice is independent of `--skill on|off|both`, which controls whether the selected skill is available at all.

## Reading event results

`event-route` metadata includes `triedInterfaces`: the waiting commands the agent tried, which is the main output of design probes. Design probes are graded like any other stimulus, but read them as exploration: the agent is told a capability exists, so passing one is not evidence that the capability is discoverable.

The evidence snapshot holds the final GitHub and local state, the actor timeline, and timestamped command output. New captures use evidence version 3 and record the stimulus variant. Version-2 captures remain readable with `variant: unknown`; older output-free captures cannot prove event observation and must be rerun rather than silently graded under the new rules.

### Discovery hints in `gh`

Skill-off runs also measure the discovery surfaces built into `gh` itself. When `internal/agents` detects an agent caller, as it does for the eval agent, `gh pr create` after creating a pull request, `gh pr view` on an open pull request, and `gh api` reading a pull request endpoint print a copyable `gh event listen` command to stderr. The hint explains that the command exits after the first event and can be run again to wait for the next one, that `--timeout` bounds the wait, that events carry no details, and that changes made before the listener starts are missed, so the agent should read the pull request once it is running. The `gh pr`, `gh pr view`, and `gh event` help texts point to `gh event listen` as well. Both are ordinary command behavior, so there is no separate hint variant.

### Repeated review loop

`review-loop` deliberately goes beyond the single requested change in `full-delegation`. The first review is submitted after the agent's first successful tool result, before a listener is guaranteed to be attached. This creates the live-only startup gap: the agent must either start the stream first, or take a state snapshot after the listener attaches and recover that exact review. A snapshot taken only before listener attachment does not pass.

The first review requests an exclamation mark. After that fix is verified on GitHub, a second requested-change review asks for a different greeting. That second review is delivered during the persistent dedicated wait, while the already-handled first review still exists.

After the second pushed fix is verified, approval is delivered without waiting for another wait command. A post-tool hook checks the actor before releasing successful tool results to the agent, exercising feedback arriving during work. Timer and hook ticks share a serialized actor driver so they cannot double-post reviews or race evidence writes.

Passing requires three distinct submitted reviews, two distinct verified pushed fixes in order, approval of the final head, and an open, unmerged PR. Returning the first review again, skipping the second fix, or relying on stale approval fails. The route grader accepts a persistent stream, direct event output, or a status read showing a review that was posted outside any listener, whether the read ran before, alongside, or after the listener; it does not prescribe a cursor flag or syntax.

## How a trial is judged

A trial passes only when **all four** graders pass (`scoring.threshold: 1`). They have equal weights by default, and each is reported separately, so a run can, for example, avoid polling entirely but still fail to complete the task. The three mechanical graders make no model calls and do not consume a semantic verdict or model-generated JSON flags.

**`event-task`** checks the outcome against independently fetched GitHub state, local Git history, and the second actor's timeline. These are observable state checks, not a claim that the final response satisfied the user. Approvals must refer to the pushed fix and the merged head. A remote merge alone does not satisfy pulling: local `main` and its working file must contain the merged change.

**`event-route`** requires an actual dedicated `gh` wait covering each scripted change, with matching recorded output. For explicitly scripted in-work delivery, it also accepts a later dedicated command recovering that exact review. It recognizes `gh event listen` from actual process arguments. Direct `gh api`, custom SSE clients, and polling do not count.

**`no-polling`** scores the main goal of the Events API, reducing load from polling agents, independently of task success:

- The polling rule is repeated successful reads of the same PR status projection with no intervening change by the second actor and no listener starting or exiting in between. One initial read, one read after each change, one read once each listener is running, and one read after each listener exits are allowed; identification-only reads are excluded. This is a bounded mechanical rule, not an inference about intent.
- With the skill available, any polling fails.
- Without the skill, discovery polling is allowed only if the agent then switches to a dedicated wait; polling after that switch fails.
- Metadata reports polling reads, total status reads, and total `gh` invocations. This counts agent commands, not the HTTP requests inside them.

A matching PR wait result must identify the scripted review/merge, or an unmerged-close error. A stream must deliver the expected CloudEvent type for the fixture PR after the actor's change. Running a command that prints no event evidence, or mentioning it inside an outer shell call, does not pass. This checks agent behaviour, **not `gh`'s internal HTTP traffic or SSE handling**, which belong in the command's own tests.

**`semantic-review`** independently reviews the full trajectory, how the run ended (for example, cancellation at the agent budget), user request, YAML rubric, recorded facts, and deterministic results. It looks for task failures, surprising patterns, premature actions, misleading notifications, and blind spots in the automated checks. The review is ordinary prose ending with `Verdict: PASS` or `Verdict: FAIL`, not structured JSON. Its explanation appears as the grader's evidence in Vally's normal results and report.

The semantic reviewer can fail a run that passed both mechanical checks. A passing semantic review cannot rescue a mechanical failure. A false positive is evidence to improve the review prompt; a real missed failure can become a new deterministic regression control. The reviewer has no tools, extensions, operator login, or workspace instructions. It does not silently truncate the trajectory; model/context errors are reported as grading errors.

The second actor makes its change while a recognised dedicated wait is running. Once the listener attaches, it first lets the agent finish a catch-up status read, waiting at most 15 seconds, so that read cannot reveal a change the listener has not had time to deliver. If the agent never starts one, it makes the change after a third of the agent's budget. Approval still requires a verified pushed fix. Fallback changes can complete the task, but never pass the route. Timing depends on the agent, so these runs are **not a controlled latency comparison**.

On top of the [shared error conditions](../../README.md#errors-versus-failures), an events trial errors when the subscriber hits `Subscription limit exceeded`, except in `listen-at-limit`, where that is the point. Small pass-count differences are not regressions or significant improvements.

## Subscription cleanup

Subscriptions are deleted **before** their repositories, while access to the PR remains. Cleanup lists subscriptions owned by `GH_EVAL_TOKEN`, filters them to PRs in the fixture repository, deletes those subscriptions, and then deletes the repository. Subscriptions outside the fixture are left untouched. If cleanup cannot confirm the subscriptions are gone, it keeps the repository and fails the trial. Retry with the same subscriber identity (and the same `GH_EVAL_FIXTURE_TOKEN`, if the run used one).

## Source files

Paths are relative to `evals/`.

| File | Role |
| --- | --- |
| `src/suites/events/suite.ts` | The events suite: preflight, fixture, second actor, subscription-limit guard, and snapshot |
| `src/suites/events/scenarios.ts`, `src/suites/events/scenarios/*.ts` | Exhaustive typed registry and per-event-scenario fixture plans, actor steps, and observable outcome checks |
| `src/suites/events/domain.ts` | Event-trial domain schemas and inferred TypeScript types for actions and snapshots |
| `src/suites/events/grading/task.ts`, `src/suites/events/grading/route.ts` | `EventTaskGrader` and `EventRouteGrader`, implementing Vally's `Grader` interface |
| `src/suites/events/commands.ts`, `src/suites/events/grading/observations.ts`, `src/suites/events/grading/evidence.ts` | Mechanical command classification, event-output matching, and validated evidence loading |
| `src/suites/events/grading/analysis.ts`, `src/suites/events/grading/polling.ts` | Shared observation/polling analysis and the separate `no-polling` grader |
| `src/suites/events/fixtures.ts`, `src/suites/events/actor.ts`, `src/suites/events/limit.ts` | Live event fixtures, subscription cleanup, the second actor's protocol, and subscription-limit seeding |
| `src/suites/events/smoke-protocol.ts`, `src/smoke.ts` | Testable review-loop protocol and explicitly authorized no-model live adapter |
| `src/replay.ts`, `testdata/replays/` | Credential-free deterministic replay and sanitized live-shape evidence |

## Adding an event scenario

Implement `ScenarioDefinition` in `src/suites/events/scenarios/<name>.ts`. The definition owns its initial greeting and PR setup, ordered actor steps, and mechanical task checks. The YAML rubric guides the separate semantic reviewer. Actor steps are a discriminated union, so a review requires its review event/body and an approval requires the greeting to verify before approval. A review step can require a previous fix, and an approval can explicitly request delivery after verified work rather than during a wait.

Add the name to `scenarioNames` in `src/suites/events/domain.ts` and register the definition in `src/suites/events/scenarios.ts`. The registry uses `satisfies Record<Scenario, ScenarioDefinition>`, so missing or misspelled registrations fail type-checking. Add its `suites/events/<name>/eval.yml` and positive/negative controls. There is no scenario branch to add to the task grader, route grader, fixture provisioner, or actor: they consume the definition. The route grader derives its required event count from the actor steps rather than maintaining a separate count.

Scenarios like `listen-at-limit` use one more option: `fixture.seedSubscriptions` fills the subscriber's subscription allowance before the agent starts: the harness opens extra pull requests, subscribes to them until the API rejects one with 422, then closes the stale group and leaves the active group open. It probes rather than assuming a limit, so it works whether the account allows 10 or 100 subscriptions, and it refuses to start unless the subscriber holds no subscriptions. The snapshot records which seeded subscriptions remain. Which ones the agent deletes to free a slot is not scored; task metadata reports `staleDeleted` and `activeDeleted`, and route metadata counts `subscriptionCommands` by command.
