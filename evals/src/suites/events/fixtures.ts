import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { ApiError, type PullRequest } from "../../core/github.js";
import { git } from "../../core/io.js";
import { cleanupSandbox, createSandbox, journalResource, type Journal } from "../../core/sandbox.js";
import { EventsGitHub, eventTypes, type EventType } from "./github.js";
import { originalGreeting, type Fixture, type Scenario, type SeededSubscriptions } from "./domain.js";
import { scenarios } from "./scenarios.js";

/** The journal kind under which event subscription IDs are recorded. */
export const subscriptionResources = "subscriptions";

export async function provision(
  github: EventsGitHub, org: string, scenario: Scenario, workDir: string,
  env: NodeJS.ProcessEnv, journalPath: string, agent = github,
): Promise<{ fixture: Fixture; journal: Journal }> {
  const plan = scenarios[scenario].fixture;
  const { repo, journal } = await createSandbox(github, org, "events", journalPath);
  await mkdir(workDir, { recursive: true });
  await git(workDir, env, "init", "--initial-branch=main");
  await git(workDir, env, "remote", "add", "origin", `https://github.com/${repo}.git`);
  await writeFile(join(workDir, "greeting.txt"), originalGreeting);
  await git(workDir, env, "add", "greeting.txt");
  await git(workDir, env, "commit", "-m", "Add greeting");
  const baseSha = await git(workDir, env, "rev-parse", "HEAD");
  await git(workDir, env, "push", "--set-upstream", "origin", "main");
  await github.api("PATCH", `/repos/${repo}`, { default_branch: "main" });
  await git(workDir, env, "switch", "-c", "improve-greeting");
  await writeFile(join(workDir, "greeting.txt"), plan.greeting);
  await git(workDir, env, "commit", "-am", "Improve greeting");
  const initialHead = await git(workDir, env, "rev-parse", "HEAD");
  const fixture: Fixture = { scenario, repo, workDir, baseSha, initialHead };
  if (plan.pullRequest === "existing") {
    await git(workDir, env, "push", "--set-upstream", "origin", "improve-greeting");
    const author = "author" in plan && plan.author === "agent" ? agent : github;
    fixture.pr = await author.api<PullRequest>("POST", `/repos/${repo}/pulls`, {
      title: "Improve greeting", head: "improve-greeting", base: "main", body: "A small greeting change.",
    });
    await git(workDir, env, "switch", "main");
  }
  if ("seedSubscriptions" in plan && plan.seedSubscriptions) {
    fixture.seeded = await seedSubscriptions(github, agent, repo, workDir, env, journal, journalPath);
  }
  return { fixture, journal };
}

/** Every non-empty set of event types, so one pull request can hold this many distinct subscriptions. */
const eventTypeSets: EventType[][] = Array.from({ length: 2 ** eventTypes.length - 1 }, (_, mask) =>
  eventTypes.filter((_, bit) => ((mask + 1) >> bit) & 1));

/** Seeding stops here even without a limit error, so a limit far above the documented one fails setup instead. */
const maxSeeded = 200;

/**
 * Fills the subscriber's subscription limit with subscriptions on other pull requests in the fixture repository, and
 * closes some of those pull requests. It creates pull requests in groups of two to close and one to keep open, and
 * spreads subscriptions across them until the API refuses one, so it works whatever the limit is. The target pull
 * request gets none: a listener matching a seeded subscription would reuse it and succeed at the limit.
 */
export async function seedSubscriptions(
  github: EventsGitHub, subscriber: EventsGitHub, repo: string, workDir: string, env: NodeJS.ProcessEnv,
  journal: Journal, journalPath: string,
): Promise<SeededSubscriptions> {
  const seeded: SeededSubscriptions = { stale: [], active: [] };
  const pulls: Array<{ pr: PullRequest; stale: boolean; sets: EventType[][] }> = [];
  const open = async (stale: boolean) => {
    const branch = `${stale ? "abandoned" : "parallel"}-change-${pulls.length + 1}`;
    await git(workDir, env, "switch", "-c", branch, "main");
    await writeFile(join(workDir, `${branch}.txt`), `${branch}\n`);
    await git(workDir, env, "add", `${branch}.txt`);
    await git(workDir, env, "commit", "-m", `Add ${branch}`);
    await git(workDir, env, "push", "--set-upstream", "origin", branch);
    await git(workDir, env, "switch", "main");
    const pr = await github.api<PullRequest>("POST", `/repos/${repo}/pulls`, {
      title: stale ? `Abandoned change ${pulls.length + 1}` : `Parallel change ${pulls.length + 1}`,
      head: branch, base: "main", body: "Another change in this repository.",
    });
    pulls.push({ pr, stale, sets: [...eventTypeSets] });
  };
  for (let count = 0; ; count++) {
    if (count >= maxSeeded) throw new Error(`The subscription limit was not reached after ${maxSeeded} subscriptions`);
    let next = pulls.filter(pull => pull.sets.length).sort((a, b) => b.sets.length - a.sets.length)[0];
    if (!next) {
      await open(true);
      await open(false);
      await open(true);
      next = pulls.at(-3)!;
    }
    const subscription = await subscriber.subscribe(next.pr.html_url, next.sets.shift()!);
    if (!subscription) break;
    await journalResource(journal, journalPath, subscriptionResources, subscription.id);
    (next.stale ? seeded.stale : seeded.active).push(subscription.id);
  }
  if (!seeded.stale.length || !seeded.active.length) {
    throw new Error("The subscription limit is too low to seed both stale and active subscriptions");
  }
  // Closing after subscribing keeps seeding independent of whether the API accepts closed pull requests.
  for (const { pr } of pulls.filter(pull => pull.stale)) {
    await github.api("PATCH", `/repos/${repo}/pulls/${pr.number}`, { state: "closed" });
  }
  return seeded;
}

/**
 * Deletes the trial's event subscriptions, then its sandbox repository. Subscriptions are created inside the agent's gh
 * process, so they are listed with the same user credential, and only those targeting PRs in the sandbox are removed.
 */
export async function cleanup(github: EventsGitHub, journal: Journal, journalPath: string, subscriber = github): Promise<void> {
  await cleanupSandbox(github, journal, journalPath, async () => {
    const prefix = `https://github.com/${journal.repo}/pull/`.toLowerCase();
    for (const subscription of await subscriber.subscriptions()) {
      const resource = subscription.resource_url.toLowerCase();
      if (!resource.startsWith(prefix)) continue;
      if (!/^\d+$/.test(resource.slice(prefix.length))) {
        throw new Error("Invalid event subscription listing");
      }
      await journalResource(journal, journalPath, subscriptionResources, subscription.id);
      try {
        await subscriber.api("DELETE", `/event-subscriptions/${subscription.id}`);
      } catch (error) {
        if (!(error instanceof ApiError) || error.status !== 404) throw error;
      }
    }
  });
}
