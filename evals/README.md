# Agent evals for `gh`

These are local, opt-in [Vally](https://microsoft.github.io/vally/) evals that give a Copilot agent a task, an already-built `gh`, and a live, private sandbox on GitHub, then grade what it did. They are meant for designing and testing `gh` features that agents use: whether the agent completes the task, and whether it takes the route the feature is meant to offer.

**`vally eval` runs them.** This directory supplies the eval definitions, a Vally executor plugin (`gh-agent`) that sets up the live GitHub side of each trial, and a grader plugin. `evals/run` is a thin wrapper that passes the right flags to the pinned `vally` CLI, so the output, results files, and re-grading are Vally's own.

The package is independent of the Go build: nothing in `go.mod`, `make`, or the normal Go tests installs or runs it. Node is required by Vally itself. Its dependencies and TypeScript compiler are confined to this directory.

## Suites

Definitions are grouped into **suites**: families of scenarios that share fixtures, an optional scripted second actor, and graders. Each suite has its own guide.

| Suite | Evaluates | Guide |
| --- | --- | --- |
| `events` | Waiting for pull request activity through `gh event` rather than polling | [suites/events/README.md](suites/events/README.md) |

To write a new one, see [Adding a suite](#adding-a-suite).

## Prerequisites

- macOS or Linux, Git, and Node.js 22.14 or newer.
- An explicitly selected Copilot model and a token with Copilot Requests access.
- An already-built `gh` binary and, for skill-on trials, the `gh` skill directory to offer.
- An explicitly authorized sandbox organization on GitHub.com. Other hosts are not supported yet.
- A user token for the evaluated `gh`. Unless a separate fixture token is supplied, it also provisions and deletes sandbox repositories, so a classic PAT needs `repo` and `delete_repo`.
- For suites with a scripted second actor, such as `events`, a **separate identity** for that actor.

Suites can need more, such as a feature flag; see their guides. Nothing uses your existing `gh` login, keychain, Copilot settings, secret stores, or default organization. Tokens are explicit inputs, and available credentials are not authorization to run against an arbitrary organization.

### Credentials

Every credential is read from an environment variable; the evals never retrieve or mint tokens. Populate them from authorized credential sources before running.

| Variable | Purpose |
| --- | --- |
| `GH_EVAL_HOST` | Must explicitly be `github.com`. |
| `GH_EVAL_ORG` | Authorized sandbox organization, such as `gh-evals-sandbox`. |
| `GH_EVAL_TOKEN` | User token for the agent's `gh`, verification, and cleanup. |
| `GH_EVAL_ACTOR_TOKEN` | Second actor identity, needed only by suites that script one. Never passed to the evaluated agent. |
| `GH_EVAL_FIXTURE_TOKEN` | Optional identity that provisions and deletes sandbox repositories, defaulting to `GH_EVAL_TOKEN`. Allows an App to manage fixtures while a user credential runs the agent. Never passed to the agent. |
| `COPILOT_GITHUB_TOKEN` | Model access for the agent and the independent semantic reviewer. |

The root `./run-evals` launcher validates these before running `evals/run`. When `GH_EVAL_ACTOR_TOKEN` is set, it also defaults fixture management to that actor identity. `--list` and `--help` need no credentials. Minting two tokens from the same App does not create two identities.

## Install and run

From the repository root:

```sh
make
npm --prefix evals ci
npm --prefix evals run build
evals/run --list
```

Then explicitly authorize live resources with `--live` and name the definitions to run:

```sh
evals/run --live --gh ./bin/gh --gh-skill ./skills/gh --model MODEL_ID DEFINITION...
```

Run `copilot` and enter `/model` to see the models available to your account.

`evals/run` calls `vally eval` once with the skill off and once with it on (`--skill on|off|both`, default `both`). The skill is offered through Vally's `--skill-dir`, not pasted into the prompt, and the runtime's own bundled skills are disabled, so the two configurations differ only by the `gh` skill. Each configuration's results land in its own directory under `~/.local/state/gh-event-evals/skill-{off,on}/<timestamp>/`. Anything after `--` goes to `vally eval` unchanged, for example `--verbose`.

To run a skill as committed at a Git revision instead of from the working tree, use `--gh-skill-ref REV:PATH` in place of `--gh-skill`. `PATH` may name the skill directory or its `SKILL.md`, as in `trunk:skills/gh` or `trunk:skills/gh/SKILL.md`. The revision must already exist locally. The runner resolves it to a commit once, exports the skill with Git, prints the pinned `COMMIT:PATH`, and records it in each trial's `runtime.log`, so a moving ref such as `trunk` stays reproducible.

Evaluate shipped skills from the repository's `skills/` directory, and compare them with an earlier version through `--gh-skill-ref` rather than a checked-in copy. To try an experimental skill that is not meant to ship, put it in `evals/skills/<experiment>/<name>/SKILL.md` and pass `--gh-skill evals/skills/<experiment>/<name>`. The directory must be named after the skill's `name`, because that is the name it is staged under; `src/skills.test.ts` checks this.

Each stimulus selects its variant with `tags.variant`, and `evals/run` runs `variant: eval` unless told otherwise (`--variant NAME`, or `--design-probe`). Trials run one at a time (`--workers 1`) and are never retried (`--max-retries 0`), since trials can share accounts and a second actor assumes it is the only change in flight. Each definition sets the agent's working budget (`constraints.max_agent_duration`) and the whole trial's timeout (`defaults.timeout`), which also covers setup, observation, and cleanup; `--agent-budget` shortens the former during iteration without editing the definition. Semantic review also consumes model quota.

While a trial runs, the executor prints progress to stderr: the fixture link, every `gh` command the agent runs, the second actor's moves, and cleanup. Vally then prints its usual per-trial report and summary.

## Reading results

Vally prints a section per trial and a summary, and saves under the results directory:

- `results.jsonl`, one record per trial, including each grader's result and metadata.
- `eval-results.md`, a readable report.
- Per-trial session directories, including the executor's artifacts: `gh-event-evidence.json`, `commands.jsonl`, `events.jsonl`, `actor.json`, `runtime.log`, and `resources.json` (the cleanup journal).

The evidence file is what graders read. It holds `{version: 3, scenario, variant, skill, calls, snapshot}`: whether the skill was offered, every observed command with timestamped stdout and stderr, and the suite's own `snapshot` of the facts its graders need. A `gh` launcher records each real `gh` process's arguments, timing, exit status, and output while forwarding its input and output. Output capture is limited to 2 MiB per command; exceeding it is an explicit observer error, not silent truncation.

Re-grade saved runs with `vally grade`, passing the same `--grader-plugin evals/dist/grader-plugin.js`: the graders read the persisted evidence, not live GitHub state. Only `semantic-review` calls the model again.

### Errors versus failures

Trials error, rather than fail, when the problem is not the agent's: setup, API, model, observer, or cleanup failures; contamination; unexpected skills offered to the agent; or a suite's `validate` guard reporting shared state that makes the run ungradable. Small pass-count differences are not regressions or significant improvements.

Harness transport failures include the GitHub request method and path. A GET reset with `ECONNRESET` is retried once with bounded backoff; mutations are not retried on an ambiguous connection reset.

An actor execution failure is not reported again as a shutdown failure. `Retry with: evals/cleanup ...` is only printed when resource cleanup itself fails or its journal cannot be read, not for unrelated agent, observer, or local-file teardown failures. A journal with `cleaned: true` confirms resource cleanup completed even if the trial errored for another reason.

## Isolation and cleanup

Each trial has a fresh home, configuration, workspace, and constructed environment. Vally runs from an empty directory, and the Copilot session has workspace configuration discovery, extensions, and the runtime's bundled skills disabled; only the selected skill is offered. `COPILOT_CLI=1` identifies the executor to `gh`'s agent detection. The agent's GitHub token lives in private temporary `gh` configuration and a Git credential helper, not `GH_TOKEN` or `GITHUB_TOKEN`. The second actor's token and grading evidence are never passed to the agent. Known token values are redacted before anything is written; treat traces as private anyway.

A trial whose agent events reveal the checkout path is an error. This is **lightweight isolation, not a security sandbox**: a determined same-user process can explore the host, read its own credentials, or bypass observation.

Every live resource is recorded in the trial's `resources.json` journal before it is created. Cleanup runs at the end of every trial, pass or fail, and deletes the sandbox repository after the suite has removed anything else it created. If it cannot confirm that, it keeps the repository and fails the trial. Each run lists trials with unfinished cleanup before and after, with a retry command for each; it never retries them automatically, because another run might still be using them. Retry with the identities the trial ran with:

```sh
GH_EVAL_TOKEN=... evals/cleanup /path/to/trial/resources.json
```

Add `GH_EVAL_FIXTURE_TOKEN=...` if the run used a separate fixture identity.

## Developing the evals

```sh
npm --prefix evals test
npm --prefix evals run check
bash -n evals/run run-evals && sh -n evals/cleanup
```

These checks use controlled trajectories, fake HTTP responses, real Git repositories in test-owned temporary directories, and `vally lint` on every definition. They use no model quota, operator accounts, or live GitHub resources. Positive and negative grader controls matter: a plausible explanation is not proof that a grader rejects an incorrect task or route.

| File | Role |
| --- | --- |
| `suites/<suite>/<scenario>/eval.yml` | Prompts, rubric, graders, scoring, and budgets |
| `src/executor-plugin.ts`, `src/grader-plugin.ts` | Vally plugin entry points registering the executor (with the suite registry) and graders |
| `src/core/executor.ts` | The `gh-agent` [executor](https://microsoft.github.io/vally/guides/writing-custom-executors/): isolated Copilot session, observer, second-actor loop, evidence, and cleanup dispatch |
| `src/core/suite.ts` | The `Suite`, `Trial`, and `TrialActor` contracts, executor config parsing, and `{{NAME}}` prompt placeholders |
| `src/core/sandbox.ts` | Private sandbox repositories and the crash-safe resource journal shared by every suite |
| `src/core/home.ts`, `src/core/observer.ts`, `src/core/session.ts` | Isolated agent home with an observed `gh`, command observation, and post-tool delivery |
| `src/core/actor.ts`, `src/core/trial-cleanup.ts` | Serialized second-actor driver and teardown orchestration that preserves execution errors |
| `src/core/calls.ts`, `src/core/github.ts`, `src/core/io.ts` | Observed command schemas and argv parsing, a minimal REST client, and shared I/O helpers |
| `src/core/errors.ts`, `src/core/evidence.ts` | `InfrastructureError` and the evidence file name |
| `src/graders/result.ts` | Helpers for deterministic grader metadata and results |
| `src/graders/review.ts`, `src/graders/copilot-review.ts` | Independent scored prose review through a tool-free Copilot session |
| `src/suites/index.ts` | The suite registry the executor and `cleanup` dispatch through |
| `src/suites/<suite>/` | Suite implementations; see each suite's guide |
| `run`, `cleanup` | Entry points shared by every suite |

## Adding a suite

The executor is not specific to any suite. It owns everything every live agent trial needs (an isolated home with an observed `gh`, skill staging, contamination and unexpected-skill guards, the trial queue, artifacts, and cleanup dispatch) and delegates the rest to a `Suite` from `src/core/suite.ts`.

### The contract

A `Suite` has:

- `name`, matching `config.suite` in its definitions and its key in `src/suites/index.ts`.
- `scenarios`, the accepted `config.scenario` values.
- `variants`, the accepted stimulus `tags.variant` values. Every stimulus needs one of them, or the trial errors with `Unsupported stimulus variant`.
- `start(context)`, which provisions one trial and returns a `Trial`. The context carries the run's credentials (`run`), the scenario, the agent's working directory and environment (for preparing a local checkout), the journal path, the agent budget, and a `progress` logger.
- An optional `cleanup`. Without it, cleanup deletes only the sandbox repository, using the fixture token. Suites that create other live resources implement it with `cleanupSandbox` and a `beforeDelete` callback that removes them.

A `Trial` has:

- `placeholders`, the values for `{{NAME}}` in stimulus prompts. A prompt that references a name the trial does not provide errors before the agent starts.
- `summary`, a progress line describing the ready fixture.
- `snapshot(observedUntil)`, which captures the facts the suite's graders need once the agent has finished. It is saved as `snapshot` in the evidence file.
- An optional `validate(rawEvents)`, which throws an `InfrastructureError` when shared state, not the agent, makes the run ungradable.
- An optional `actor`, a scripted second actor. The executor ticks it on a timer, and also after each successful tool when `postTool` is set; ticks are serialized. `trigger(calls, now)` picks the running agent call the actor may act during (`triggerNote` is logged when a new one is recognised), `tick` acts, `actions` and `done` report progress, and `describe` turns an action into a progress line. The actor's token is `run.actorToken`; suites that need one should refuse to start without it.

Use `createSandbox` from `src/core/sandbox.ts` for the private repository; it journals the repository before creating it. Record any other live resource with `journalResource(journal, journalPath, kind, id)` before creating it, so an interrupted trial can still be cleaned up.

### A minimal suite

`src/suites/readme/suite.ts`:

```ts
import { GitHub } from "../../core/github.js";
import { createSandbox } from "../../core/sandbox.js";
import type { Suite } from "../../core/suite.js";

/** The readme suite: the agent answers questions about a sandbox repository's README. */
export const readmeSuite: Suite = {
  name: "readme",
  scenarios: ["summarize"],
  variants: ["eval"],
  async start({ run, journalPath }) {
    const github = new GitHub(run.fixtureToken, run.request);
    const { repo } = await createSandbox(github, run.org, "readme", journalPath);
    await github.api("PUT", `/repos/${repo}/contents/README.md`, {
      message: "Add README", content: Buffer.from("# Greeter\n\nSays hello.\n").toString("base64"),
    });
    return {
      placeholders: { REPO: repo },
      summary: `Sandbox ready: https://github.com/${repo}`,
      snapshot: async () => ({ repo }),
    };
  },
};
```

Register it in `src/suites/index.ts` as `readme: readmeSuite`.

`src/suites/readme/grader.ts` reads the evidence file from the trial's artifact directory and validates the parts it uses:

```ts
import type { Grader, GraderInput, GraderResult } from "@microsoft/vally";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { z } from "zod";
import { callSchema } from "../../core/calls.js";
import { evidenceFile } from "../../core/evidence.js";
import { metadata, result } from "../../graders/result.js";

const evidenceSchema = z.object({
  version: z.literal(3),
  calls: z.array(callSchema),
  snapshot: z.object({ repo: z.string() }),
});

/** Passes when a successful gh command read the sandbox repository. */
export class ReadmeRouteGrader implements Grader {
  readonly metadata = metadata("readme-route");

  async grade(input: GraderInput): Promise<GraderResult> {
    const dir = input.trajectory?.artifactDir;
    if (!dir) throw new Error("readme-route needs a trial run by the gh-agent executor");
    const evidence = evidenceSchema.parse(JSON.parse(await readFile(join(dir, evidenceFile), "utf8")));
    const read = evidence.calls.some(call =>
      call.kind === "gh" && call.success === true && call.arguments.includes(evidence.snapshot.repo));
    return result("readme-route", [[read, "No successful gh command read the sandbox repository"]]);
  }
}
```

Register it in `registerGraders` in `src/grader-plugin.ts` with `registry.register(new ReadmeRouteGrader())`.

`suites/readme/summarize/eval.yml`:

```yaml
name: summarize
description: The agent summarizes a sandbox repository's README using gh.
defaults:
  executor:
    name: gh-agent
    config:
      suite: readme
      scenario: summarize
  runs: 1
  timeout: 5m
scoring:
  threshold: 1
stimuli:
  - name: summarize
    tags:
      variant: eval
    constraints:
      max_agent_duration: 2m
    prompt: "What does the README of {{REPO}} say?"
    graders:
      - type: readme-route
```

Vally's built-in graders, such as `program` and `tool-call`, also work unchanged. Grading policies belong to their suite: `event-route`, for example, is specifically the policy of waiting through events rather than polling, and its evidence and timing rules stay under `src/suites/events/`. No scenario switch belongs in a universal route grader.

Add positive and negative controls for each grader, and a `vally lint` check for each definition, alongside the existing ones in `src/graders.test.ts`.

### Current limits

These are planned to go away; until then, a new suite works around them:

- `evals/run` lists and runs only `suites/events` definitions. Run other suites with Vally directly, from an empty directory, with the same safety flags:

  ```sh
  export GH_EVAL_GH="$PWD/bin/gh"
  (cd "$(mktemp -d)" && "$OLDPWD/evals/node_modules/.bin/vally" eval \
    --eval-spec "$OLDPWD/evals/suites/readme/summarize/eval.yml" \
    --executor-plugin "$OLDPWD/evals/dist/executor-plugin.js" \
    --grader-plugin "$OLDPWD/evals/dist/grader-plugin.js" \
    --model MODEL_ID --tag variant=eval --workers 1 --max-retries 0 --shutdown-timeout 5m)
  ```

  Add `--skill-dir DIR` to offer a skill. `GH_EVAL_GH` must be an absolute path. Run `node evals/dist/cleanup.js --unfinished RESULTS_DIR` afterwards to list trials whose cleanup did not finish.
- The evidence file is named `gh-event-evidence.json` for every suite, and its envelope does not record the suite. Its `scenario` and `snapshot` are only meaningful within the suite that wrote it.
- `semantic-review` is registered with the events evidence and checks only, so other suites cannot select it yet.
- `evals/replay` and `evals/smoke` are specific to the events suite.
