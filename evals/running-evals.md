# Running the event evals

Run these commands from the repository root. The evals create and delete real
private repositories in the authorized sandbox organization.

For execution and interpretation, use
[`running-evals`](../.github/skills/running-evals/SKILL.md).

## Prepare the candidate and eval plugins

```sh
make
npm --prefix evals ci
npm --prefix evals run build
```

Credentials come only from environment variables. Populate them in the environment of the process that runs the eval before starting it; nothing in this repository retrieves or mints tokens. Do not source the evaluated GitHub token from the operator's default `gh` login, and do not paste tokens into chat.

```sh
export GH_EVAL_HOST=github.com
export GH_EVAL_ORG=gh-evals-sandbox
export GH_EVAL_TOKEN=...
export GH_EVAL_ACTOR_TOKEN=...
export COPILOT_GITHUB_TOKEN=...
```

`GH_EVAL_TOKEN` is the preview-enabled user credential used by the evaluated `gh` and event subscriptions. `COPILOT_GITHUB_TOKEN` is only for model access. The events suite also needs `GH_EVAL_ACTOR_TOKEN`, a separate reviewer and merger identity such as a sandbox App installation token. `GH_EVAL_FIXTURE_TOKEN` optionally selects the identity that creates and deletes sandbox repositories.

Use the root launcher, which validates the tokens and defaults `GH_EVAL_FIXTURE_TOKEN` to the actor token before running `evals/run`:

```sh
RUN_EVALS=./run-evals
```

Use `RUN_EVALS=evals/run` only when every required token, including any fixture token, is already exported.

The examples below use:

```sh
COMMON=(--live --gh ./bin/gh --model gpt-5.6-luna)
```

Use `caffeinate -i` before `"$RUN_EVALS"` on macOS if the machine may sleep.

## List scenarios

```sh
"$RUN_EVALS" --list
```

| Scenario | Behavior under evaluation |
| --- | --- |
| `notify-review` | Wait for and report an incoming submitted review. |
| `pull-after-merge` | Wait for a merge, then update local `main`. |
| `pull-after-close` | Detect close-without-merge and leave local `main` unchanged. |
| `full-delegation` | Create a PR, address feedback, await approval, and merge. |
| `review-loop` | Address two successive review rounds and await final approval without merging. |
| `listen-at-limit` | Asked to wait for a review while the subscription limit is filled: free a slot, listen, and report the review without polling. |

`listen-at-limit` fills the subscriber's allowance before the agent starts, so the subscriber must hold no subscriptions; the runner errors otherwise. It is not in the core matrix below; run it on its own:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill both \
  --gh-skill ./skills/gh \
  listen-at-limit
```

## Core skill-off and skill-on cases

Run a skill-off baseline:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill off \
  review-loop
```

Run with the event-aware `gh` skill:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill on \
  --gh-skill ./skills/gh \
  review-loop
```

Run the off/on comparison sequentially:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill both \
  --gh-skill ./skills/gh \
  review-loop
```

Run the complete core matrix:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill both \
  --gh-skill ./skills/gh \
  notify-review \
  pull-after-merge \
  pull-after-close \
  full-delegation \
  review-loop
```

The intended first-pass result is a normal graded failure without the skill and
a 4/4 pass with the event-aware skill. Infrastructure or cleanup errors are not
valid baseline failures.

## Skills from a Git revision

Use `--gh-skill-ref REV:PATH` instead of `--gh-skill` to take the skill from a commit rather than the working tree. `PATH` is a skill directory or its `SKILL.md`, so `trunk:skills/gh` and `trunk:skills/gh/SKILL.md` both work. The revision must exist locally; the runner never fetches. It resolves the revision to a commit once, prints the pinned `COMMIT:PATH`, and records it in each trial's `runtime.log`.

Compare the shipped skill against an earlier version, for example the parent of the commit that changed it:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill on \
  --gh-skill-ref COMMIT~1:skills/gh \
  review-loop
```

## Design probes

Design probes tell the agent that an undocumented event-driven capability
exists without supplying command syntax:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill off \
  --design-probe \
  notify-review
```

Run all design probes:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill off \
  --design-probe \
  notify-review pull-after-merge pull-after-close full-delegation review-loop
```

A design-probe pass shows that the interface can be discovered from a capability
hint. It does not prove that the ordinary user prompt is sufficient.

## Short development runs

The checked-in agent budget is five minutes. Override it only for development:

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill on \
  --gh-skill ./skills/gh \
  --agent-budget 2m \
  review-loop
```

Omit `--agent-budget` for final measurements.

## Repeat trials

```sh
"$RUN_EVALS" "${COMMON[@]}" \
  --skill both \
  --gh-skill ./skills/gh \
  --runs 3 \
  notify-review
```

Trials are serialized because they share a subscriber account and actor timing.

## Offline deterministic replay

Replay the checked-in sanitized evidence:

```sh
npm --prefix evals run build
evals/replay evals/testdata/replays/persistent-review-loop.json
```

Replay a saved executor artifact directory:

```sh
evals/replay \
  ~/.local/state/gh-event-evals/skill-on/<run>/<scenario>/<stimulus>/<model>/0/executor-artifacts
```

Replay runs `event-task`, `event-route`, and `no-polling` without credentials,
network access, live resources, or a model. Semantic review is intentionally not
re-run.

## No-model live smoke

The smoke test exercises the real candidate command and live API without an
agent model:

```sh
GH_EVAL_HOST=github.com \
GH_EVAL_ORG=gh-evals-sandbox \
GH_EVAL_TOKEN=... \
GH_EVAL_ACTOR_TOKEN=... \
GH_EVAL_FIXTURE_TOKEN=... \
evals/smoke --live --gh ./bin/gh
```

After the review loop, the smoke creates, views, lists, and deletes one extra subscription with `gh event subscription`.

The smoke requires the actor token directly; `evals/run` and `./run-evals` cannot be substituted for `evals/smoke`.

## Results and cleanup

Results are written under:

```text
~/.local/state/gh-event-evals/skill-off/<timestamp>/
~/.local/state/gh-event-evals/skill-on/<timestamp>/
```

Useful files include `eval-results.md`, `results.jsonl`, `commands.jsonl`,
`events.jsonl`, `gh-event-evidence.json`, and `resources.json`.

Normal trials clean up automatically. If a trial reports an unfinished cleanup,
retry with the same credential identities:

```sh
GH_EVAL_TOKEN=... \
GH_EVAL_FIXTURE_TOKEN=... \
evals/run --live --cleanup /path/to/resources.json
```

Do not use a different user token to clean up event subscriptions: subscription
ownership is user-specific.
