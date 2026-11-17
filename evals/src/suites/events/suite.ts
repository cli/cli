import { InfrastructureError } from "../../core/errors.js";
import { containsCommit, git } from "../../core/io.js";
import type { Call } from "../../core/calls.js";
import type { Suite, Trial, TrialActor, TrialContext } from "../../core/suite.js";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { Actor } from "./actor.js";
import { dedicatedWait, readyForActor } from "./commands.js";
import type { Action, Fixture, Scenario, Snapshot } from "./domain.js";
import { cleanup, provision } from "./fixtures.js";
import { EventsGitHub } from "./github.js";
import { subscriptionLimitHit } from "./limit.js";
import { scenarios } from "./scenarios.js";
import type { ScenarioDefinition } from "./scenarios/definition.js";

function describe(action: Action): string {
  return `${action.kind} (${action.trigger === "fallback"
    ? "fallback: no dedicated wait was running" : action.trigger === "wait"
      ? "during a dedicated wait" : action.trigger === "after-tool"
        ? "after the first tool, before listener attachment was guaranteed"
        : action.trigger === "after-fix" ? "after verified work" : "verified"})`;
}

async function start(context: TrialContext): Promise<Trial> {
  const { run, workDir, env, journalPath, progress } = context;
  const scenario = context.scenario as Scenario;
  if (!run.actorToken) throw new InfrastructureError("GH_EVAL_ACTOR_TOKEN must be supplied explicitly");
  const github = new EventsGitHub(run.fixtureToken, run.request);
  const subscriber = new EventsGitHub(run.token, run.request);
  const definition: ScenarioDefinition = scenarios[scenario];
  const seeding = definition.fixture.seedSubscriptions === true;
  // Seeded subscriptions must be the only ones, so the grader knows what each deletion removed.
  if (seeding && (await subscriber.subscriptions()).length) {
    throw new InfrastructureError("The GH_EVAL_TOKEN user must start this scenario with no event subscriptions. "
      + "Clean up leftover subscriptions before running again.");
  }
  progress("creating sandbox repository and fixture");
  const { fixture } = await provision(github, run.org, scenario, workDir, env, journalPath, subscriber);
  if (fixture.seeded) {
    progress(`seeded ${fixture.seeded.stale.length} stale and ${fixture.seeded.active.length} active subscriptions`);
  }
  const actor = new Actor(
    github, new EventsGitHub(run.actorToken, run.request), fixture, context.agentBudgetMs / 3, Date.now());
  const trialActor: TrialActor<Action> = {
    get done() { return actor.done; },
    actions: actor.actions,
    postTool: definition.steps.some(step => "delivery" in step
      && (step.delivery === "after-tool" || step.delivery === "after-fix")),
    trigger: (calls, now) => calls.find(call => call.endedAt === undefined && dedicatedWait(call) !== undefined
      && readyForActor(call, calls as Call[], now)),
    triggerNote: "recognised an attached dedicated gh wait from its recorded argv and stderr",
    tick: (now, trigger, source) => actor.tick(now, trigger, source),
    describe,
  };
  return {
    placeholders: fixture.pr ? { PR_URL: fixture.pr.html_url } : {},
    summary: fixture.pr?.html_url ?? `https://github.com/${fixture.repo} (agent creates the PR)`,
    actor: trialActor as TrialActor,
    validate(rawEvents) {
      if (!seeding && subscriptionLimitHit(rawEvents as unknown[])) {
        throw new InfrastructureError("The GH_EVAL_TOKEN user hit the event subscription limit, so this "
          + "trial cannot be graded. Clean up leftover subscriptions before running again.");
      }
    },
    snapshot: observedUntil => captureSnapshot(github, subscriber, fixture, workDir, env, actor.actions, observedUntil),
  };
}

async function captureSnapshot(
  github: EventsGitHub, subscriber: EventsGitHub, fixture: Pick<Fixture, "repo" | "baseSha" | "pr" | "seeded">,
  workDir: string, env: NodeJS.ProcessEnv, actions: Snapshot["actions"], observedUntil: number,
): Promise<Snapshot> {
  const remaining = fixture.seeded ? new Set((await subscriber.subscriptions()).map(subscription => subscription.id)) : undefined;
  const pr = fixture.pr ? await github.pull(fixture.repo, fixture.pr.number) : undefined;
  let localGreeting: string | undefined;
  try { localGreeting = await readFile(join(workDir, "greeting.txt"), "utf8"); }
  catch (error) {
    if (!(error instanceof Error && "code" in error && error.code === "ENOENT")) throw error;
  }
  return {
    observedUntil, pr, actions, localGreeting,
    ...(fixture.seeded && remaining ? { subscriptions: {
      ...fixture.seeded,
      remaining: [...fixture.seeded.stale, ...fixture.seeded.active].filter(id => remaining.has(id)),
    } } : {}),
    reviews: pr ? await github.reviews(fixture.repo, pr.number) : [],
    greeting: pr ? await github.greeting(fixture.repo, pr.merged && pr.merge_commit_sha ? pr.merge_commit_sha : pr.head.sha) : undefined,
    currentBranch: await git(workDir, env, "branch", "--show-current"),
    mainContainsMerge: pr?.merge_commit_sha && pr.merged
      ? await containsCommit(workDir, env, pr.merge_commit_sha)
      : await git(workDir, env, "rev-parse", "refs/heads/main") !== fixture.baseSha,
  };
}

/** The event-streaming suite: PR fixtures, a second actor that reviews or merges, and event subscription cleanup. */
export const eventsSuite: Suite = {
  name: "events",
  scenarios: Object.keys(scenarios),
  variants: ["eval", "design-probe", "raw-api"],
  start,
  cleanup: (journal, journalPath, { token, fixtureToken, request }) =>
    cleanup(new EventsGitHub(fixtureToken, request), journal, journalPath, new EventsGitHub(token, request)),
};
