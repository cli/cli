import assert from "node:assert/strict";
import { test, type TestContext } from "node:test";
import { mkdir, mkdtemp, writeFile, readFile, rm, symlink, access } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { spawn, execFile } from "node:child_process";
import { promisify } from "node:util";
import { Actor } from "./suites/events/actor.js";
import { EventsGitHub } from "./suites/events/github.js";
import { cleanup, provision, seedSubscriptions } from "./suites/events/fixtures.js";
import { parseJournal, unfinishedJournals, type Journal } from "./core/sandbox.js";
import { containsCommit, git, isolatedEnvironment, redact } from "./core/io.js";
import { CommandObserver, contamination, observe, writeGhWrapper } from "./core/observer.js";
import { subscriptionLimitHit } from "./suites/events/limit.js";
import { effectiveAgentBudget, stimulusVariant } from "./core/executor.js";
import { prepareHome } from "./core/home.js";
import { eventsSuite } from "./suites/events/suite.js";
import { cleanupSuite, type Suite } from "./core/suite.js";
import { reviewedGreeting, type Fixture } from "./suites/events/domain.js";
import { type Call } from "./core/calls.js";

const exec = promisify(execFile);

async function temporary(t: TestContext): Promise<string> {
  const path = await mkdtemp(join(tmpdir(), "gh-test-"));
  t.after(() => rm(path, { recursive: true }));
  return path;
}

async function observedProcess(t: TestContext, source: string) {
  const root = await temporary(t);
  const binary = join(root, "real");
  const wrapper = join(root, "gh");
  await writeFile(binary, `#!${process.execPath}\n${source}\n`, { mode: 0o700 });
  const calls: Call[] = [];
  const observer = new CommandObserver(calls, () => {});
  await observer.listen(join(root, ".sock"));
  await writeGhWrapper(wrapper, binary, join(root, ".sock"));
  const child = spawn(wrapper, ["event", "listen"], {
    env: isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused"),
  });
  const completed = new Promise<number | null>((resolve, reject) => {
    child.on("error", reject);
    child.on("close", resolve);
  });
  t.after(async () => {
    if (child.exitCode === null && child.signalCode === null) child.kill("SIGTERM");
    await completed;
    await observer.close();
  });
  return { calls, observer, child, completed };
}

interface Expected {
  method: string;
  path: string;
  response?: unknown;
  status?: number;
  check?: (body: Record<string, unknown>) => void;
}

function api(t: TestContext, expected: Expected[]): EventsGitHub {
  const queue = [...expected];
  t.after(() => assert.equal(queue.length, 0, "Unused expected API requests"));
  return new EventsGitHub("fixture-token", async (input, init) => {
    const next = queue.shift();
    assert.ok(next, `Unexpected request ${init?.method} ${String(input)}`);
    assert.equal(init?.method, next.method);
    assert.equal(String(input), `https://api.github.com${next.path}`);
    assert.equal(init?.redirect, "error");
    next.check?.(JSON.parse(String(init?.body)));
    const status = next.status ?? 200;
    return new Response(status === 204 ? null : JSON.stringify(next.response), { status });
  });
}

test("fixture cleanup uses the subscriber credential only for subscriptions", async t => {
  const root = await temporary(t);
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  const pr = `https://github.com/${repo}/pull/1`;
  const fixtureApi = api(t, [
    { method: "GET", path: `/repos/${repo}`, response: { id: 1, private: true } },
    { method: "DELETE", path: `/repos/${repo}`, status: 204 },
  ]);
  const subscriber = api(t, [
    { method: "GET", path: "/event-subscriptions?per_page=100",
      response: { subscriptions: [{ id: "sub_fixture", resource_url: pr, event_types: [] }] } },
    { method: "DELETE", path: "/event-subscriptions/sub_fixture", status: 204 },
  ]);
  const journal: Journal = { version: 2, suite: "events", repo, id: 1, cleaned: false, resources: {} };
  await cleanup(fixtureApi, journal, join(root, "resources.json"), subscriber);
  assert.equal(journal.cleaned, true);
});

function fixture(): Fixture {
  return {
    scenario: "full-delegation", repo: "sandbox/run", workDir: "/unused",
    baseSha: "base", initialHead: "initial",
    pr: {
      number: 1, html_url: "https://github.com/sandbox/run/pull/1", state: "open", merged: false, mergeable: true,
      merge_commit_sha: null, head: { sha: "initial", ref: "improve-greeting" }, base: { ref: "main" },
    },
  };
}

test("actor waits for readiness, requests a fix, and approves only the verified pushed head", async t => {
  const data = fixture();
  const fixed = { ...data.pr!, head: { sha: "fixed", ref: "improve-greeting" } };
  const github = api(t, [
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: data.pr },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: fixed },
    { method: "GET", path: "/repos/sandbox/run/contents/greeting.txt?ref=fixed", response: { encoding: "base64", content: Buffer.from("still wrong\n").toString("base64") } },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: fixed },
    { method: "GET", path: "/repos/sandbox/run/contents/greeting.txt?ref=fixed", response: { encoding: "base64", content: Buffer.from(reviewedGreeting).toString("base64") } },
  ]);
  const reviewer = api(t, [
    { method: "POST", path: "/repos/sandbox/run/pulls/1/reviews", response: { id: 11 },
      check: body => { assert.equal(body.event, "REQUEST_CHANGES"); assert.equal(body.commit_id, "initial"); } },
    { method: "POST", path: "/repos/sandbox/run/pulls/1/reviews", response: { id: 12 },
      check: body => { assert.equal(body.event, "APPROVE"); assert.equal(body.commit_id, "fixed"); } },
  ]);
  const actor = new Actor(github, reviewer, data, 100, 0);
  await actor.tick(10, { id: "first", kind: "gh", output: [], arguments: [], startedAt: 5 });
  assert.equal(actor.phase, "approval");
  await actor.tick(20);
  assert.deepEqual(actor.actions.map(action => action.kind), ["review"]);
  await actor.tick(30, { id: "second", kind: "gh", output: [], arguments: [], startedAt: 25 });
  assert.deepEqual(actor.actions.map(action => action.kind), ["review", "fix", "approval"]);
  assert.equal(actor.actions[2]!.head, "fixed");
});

test("the actor fallback emits a change even if the agent never discovers waiting", async t => {
  const data = fixture();
  data.scenario = "notify-review";
  const github = api(t, [
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: data.pr },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: data.pr },
  ]);
  const reviewer = api(t, [{
    method: "POST", path: "/repos/sandbox/run/pulls/1/reviews", response: { id: 9 },
    check: body => {
      assert.equal(body.event, "COMMENT");
      assert.equal(body.body, "I've reviewed this change. The greeting looks good.");
    },
  }]);
  const actor = new Actor(github, reviewer, data, 100, 0);
  await actor.tick(99);
  assert.equal(actor.actions.length, 0);
  await actor.tick(100);
  assert.equal(actor.actions[0]!.trigger, "fallback");
});

test("the actor allows asynchronous mergeability computation without a flaky merge attempt", async t => {
  const data = fixture();
  data.scenario = "pull-after-merge";
  const github = api(t, [
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: { ...data.pr, mergeable: null } },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: { ...data.pr, mergeable: true } },
  ]);
  const reviewer = api(t, [{
    method: "PUT", path: "/repos/sandbox/run/pulls/1/merge", response: { merged: true },
    check: body => assert.equal(body.sha, "initial"),
  }]);
  const actor = new Actor(github, reviewer, data, 100, 0);
  const wait: Call = { id: "waiting", kind: "gh", output: [], arguments: [], startedAt: 1 };
  await actor.tick(10, wait);
  assert.equal(actor.actions.length, 0);
  await actor.tick(20, wait);
  assert.equal(actor.actions[0]!.kind, "merge");
});

test("the close scenario closes without merging during the observed wait", async t => {
  const data = fixture();
  data.scenario = "pull-after-close";
  const github = api(t, [{ method: "GET", path: "/repos/sandbox/run/pulls/1", response: data.pr }]);
  const reviewer = api(t, [{
    method: "PATCH", path: "/repos/sandbox/run/pulls/1", response: {},
    check: body => assert.deepEqual(body, { state: "closed" }),
  }]);
  const actor = new Actor(github, reviewer, data, 100, 0);
  await actor.tick(10, { id: "waiting", kind: "gh", output: [], arguments: [], startedAt: 1 });
  assert.equal(actor.done, true);
  assert.deepEqual(actor.actions, [{
    kind: "close", at: 10, pr: 1, head: "initial", trigger: "wait", callId: "waiting",
  }]);
});

test("approval fallback starts after fix verification, not while the agent is editing", async t => {
  const data = fixture();
  const fixed = { ...data.pr!, head: { sha: "fixed", ref: "topic" } };
  const github = api(t, [
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: data.pr },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: fixed },
    { method: "GET", path: "/repos/sandbox/run/contents/greeting.txt?ref=fixed", response: {
      encoding: "base64", content: Buffer.from(reviewedGreeting).toString("base64"),
    } },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: fixed },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: fixed },
  ]);
  const reviewer = api(t, [
    { method: "POST", path: "/repos/sandbox/run/pulls/1/reviews", response: { id: 1 } },
    { method: "POST", path: "/repos/sandbox/run/pulls/1/reviews", response: { id: 2 } },
  ]);
  const actor = new Actor(github, reviewer, data, 100, 0);
  await actor.tick(10, { id: "first", kind: "gh", output: [], arguments: [], startedAt: 1 });
  await actor.tick(110);
  await actor.tick(209);
  assert.deepEqual(actor.actions.map(action => action.kind), ["review", "fix"]);
  await actor.tick(210);
  assert.equal(actor.actions.at(-1)!.kind, "approval");
  assert.equal(actor.actions.at(-1)!.trigger, "fallback");
});

test("approval never reuses a fix verification for a different pushed head", async t => {
  const data = fixture();
  const fixed = { ...data.pr!, head: { sha: "fixed", ref: "topic" } };
  const changed = { ...data.pr!, head: { sha: "changed", ref: "topic" } };
  const github = api(t, [
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: data.pr },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: fixed },
    { method: "GET", path: "/repos/sandbox/run/contents/greeting.txt?ref=fixed", response: {
      encoding: "base64", content: Buffer.from(reviewedGreeting).toString("base64"),
    } },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: changed },
    { method: "GET", path: "/repos/sandbox/run/contents/greeting.txt?ref=changed", response: {
      encoding: "base64", content: Buffer.from("incorrect again\n").toString("base64"),
    } },
  ]);
  const reviewer = api(t, [{ method: "POST", path: "/repos/sandbox/run/pulls/1/reviews", response: { id: 1 } }]);
  const actor = new Actor(github, reviewer, data, 100, 0);
  await actor.tick(10, { id: "first", kind: "gh", output: [], arguments: [], startedAt: 1 });
  await actor.tick(20);
  await actor.tick(30, { id: "second", kind: "gh", output: [], arguments: [], startedAt: 25 });
  assert.deepEqual(actor.actions.map(action => action.kind), ["review", "fix"]);
});

test("finding a newly created PR does not discard an already-running wait", async t => {
  const data = fixture();
  const pr = data.pr;
  delete data.pr;
  const github = api(t, [
    { method: "GET", path: "/repos/sandbox/run/pulls?state=all&base=main&per_page=100", response: [{ number: 1 }] },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: pr },
    { method: "GET", path: "/repos/sandbox/run/pulls/1", response: pr },
  ]);
  const reviewer = api(t, [{ method: "POST", path: "/repos/sandbox/run/pulls/1/reviews", response: { id: 3 } }]);
  const actor = new Actor(github, reviewer, data, 100, 0);
  await actor.tick(10, { id: "waiting", kind: "gh", output: [], arguments: [], startedAt: 5 });
  assert.equal(actor.actions[0]!.trigger, "wait");
});

test("cleanup lists subscriptions and deletes only fixture subscriptions before the repository", async t => {
  const root = await temporary(t);
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  const own = `https://github.com/${repo}/pull/1`;
  const other = "https://github.com/sandbox/other/pull/2";
  const fixture = api(t, [
    { method: "GET", path: `/repos/${repo}`, response: { id: 1, private: true } },
    { method: "DELETE", path: `/repos/${repo}`, status: 204 },
  ]);
  const subscriber = api(t, [
    { method: "GET", path: "/event-subscriptions?per_page=100", response: { subscriptions: [
      { id: "sub_own", resource_url: own, event_types: ["review"] },
      { id: "sub_other", resource_url: other, event_types: ["review"] },
    ] } },
    { method: "DELETE", path: "/event-subscriptions/sub_own", status: 204 },
  ]);
  const journal: Journal = { version: 2, suite: "events", repo, id: 1, cleaned: false, resources: {} };
  await cleanup(fixture, journal, join(root, "resources.json"), subscriber);
  assert.equal(journal.cleaned, true);
  assert.deepEqual(journal.resources.subscriptions, ["sub_own"]);
  await cleanup(fixture, journal, join(root, "resources.json"), subscriber);
  assert.equal(JSON.parse(await readFile(join(root, "resources.json"), "utf8")).cleaned, true);
});

test("cleanup can be retried after a repository deletion whose outcome was not recorded", async t => {
  const root = await temporary(t);
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  const subscriber = api(t, [
    { method: "GET", path: "/event-subscriptions?per_page=100", response: { subscriptions: [] } },
  ]);
  const journal: Journal = { version: 2, suite: "events", repo, id: 1, cleaned: false, resources: {} };
  await cleanup(api(t, [
    { method: "GET", path: `/repos/${repo}`, response: { id: 1, private: true } },
    { method: "DELETE", path: `/repos/${repo}`, status: 404, response: { message: "Not Found" } },
  ]), journal, join(root, "resources.json"), subscriber);
  assert.equal(journal.cleaned, true, "a retried DELETE that finds the repository gone has deleted it");

  const interrupted: Journal = { version: 2, suite: "events", repo, id: 1, cleaned: false, deleting: true, resources: {} };
  await cleanup(api(t, [
    { method: "GET", path: `/repos/${repo}`, status: 404, response: { message: "Not Found" } },
  ]), interrupted, join(root, "resources.json"));
  assert.equal(JSON.parse(await readFile(join(root, "resources.json"), "utf8")).cleaned, true);

  await assert.rejects(cleanup(api(t, [
    { method: "GET", path: `/repos/${repo}`, status: 404, response: { message: "Not Found" } },
  ]), { version: 2, suite: "events", repo, id: 1, cleaned: false, resources: {} }, join(root, "resources.json")),
  /cleanup cannot be confirmed/, "a missing repository whose deletion never started is not assumed deleted");
});

test("cleanup succeeds when the subscriber has no fixture subscriptions", async t => {
  const root = await temporary(t);
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  const fixture = api(t, [
    { method: "GET", path: `/repos/${repo}`, response: { id: 1, private: true } },
    { method: "DELETE", path: `/repos/${repo}`, status: 204 },
  ]);
  const subscriber = api(t, [
    { method: "GET", path: "/event-subscriptions?per_page=100", response: { subscriptions: [] } },
  ]);
  const journal: Journal = { version: 2, suite: "events", repo, id: 1, cleaned: false, resources: {} };
  await cleanup(fixture, journal, join(root, "resources.json"), subscriber);
  assert.equal(journal.cleaned, true);
});

test("cleanup refuses a malformed subscription listing", async t => {
  const root = await temporary(t);
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  const fixture = api(t, [
    { method: "GET", path: `/repos/${repo}`, response: { id: 1, private: true } },
  ]);
  const subscriber = api(t, [
    { method: "GET", path: "/event-subscriptions?per_page=100", response: { subscriptions: [
      { id: "invalid", resource_url: `https://github.com/${repo}/pull/1`, event_types: [] },
    ] } },
  ]);
  await assert.rejects(cleanup(fixture, { version: 2, suite: "events", repo, id: 1, cleaned: false, resources: {} },
    join(root, "resources.json"), subscriber), /Invalid event subscription listing/);
});

test("a cleanup rate limit is explicit and leaves the repository accessible", async t => {
  const root = await temporary(t);
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  const pr = `https://github.com/${repo}/pull/1`;
  const fixture = api(t, [
    { method: "GET", path: `/repos/${repo}`, response: { id: 1, private: true } },
  ]);
  const subscriber = api(t, [
    { method: "GET", path: "/event-subscriptions?per_page=100",
      response: { subscriptions: [{ id: "sub_own", resource_url: pr, event_types: [] }] } },
    { method: "DELETE", path: "/event-subscriptions/sub_own", status: 403, response: { message: "You have exceeded a secondary rate limit" } },
  ]);
  const journal: Journal = { version: 2, suite: "events", repo, id: 1, cleaned: false, resources: {} };
  await assert.rejects(cleanup(fixture, journal, join(root, "resources.json"), subscriber),
    /HTTP 403.*secondary rate limit/);
  assert.equal(journal.cleaned, false);
  assert.deepEqual(journal.resources.subscriptions, ["sub_own"]);
});

test("subscription-limit refusals in the agent's tool output are detected as infrastructure, not agent failure", () => {
  const limit = { type: "tool.execution_complete", data: { toolCallId: "a", success: false,
    result: { content: "gh: HTTP 422: Subscription limit exceeded" } } };
  assert.equal(subscriptionLimitHit([limit]), true);
  assert.equal(subscriptionLimitHit([{ type: "assistant.message", data: { content: "Subscription limit exceeded" } }]), false);
  assert.equal(subscriptionLimitHit([{ type: "tool.execution_complete", data: { result: "ok" } }]), false);
});

test("unfinished journals are reported, while cleaned or absent journals are not", async t => {
  const root = await temporary(t);
  const journal = (cleaned: boolean): Journal => ({
    version: 2, suite: "events", repo: "sandbox/run-00000000-0000-4000-8000-000000000001", cleaned, resources: {},
  });
  await mkdir(join(root, "run-a", "notify-review-skill-off-1"), { recursive: true });
  await mkdir(join(root, "run-a", "notify-review-skill-on-1"), { recursive: true });
  await mkdir(join(root, "run-b", "pull-after-merge-skill-off-1"), { recursive: true });
  await mkdir(join(root, "run-b", "no-journal"), { recursive: true });
  await writeFile(join(root, "run-a", "notify-review-skill-off-1", "resources.json"), JSON.stringify(journal(true)));
  await writeFile(join(root, "run-a", "notify-review-skill-on-1", "resources.json"), JSON.stringify(journal(false)));
  await writeFile(join(root, "run-b", "pull-after-merge-skill-off-1", "resources.json"), "not json");
  await writeFile(join(root, "stray-file"), "ignored");
  const nested = join(root, "skill-on", "2026-10-29T15-22-33-723Z", "notify-review", "notify-review",
    "gpt-5.6-luna", "trial-1", "executor-artifacts");
  await mkdir(nested, { recursive: true });
  await writeFile(join(nested, "resources.json"), JSON.stringify(journal(false)));
  assert.deepEqual(await unfinishedJournals(root), [
    join(root, "run-a", "notify-review-skill-on-1", "resources.json"),
    join(root, "run-b", "pull-after-merge-skill-off-1", "resources.json"),
    join(nested, "resources.json"),
  ]);
  assert.deepEqual(await unfinishedJournals(join(root, "missing")), []);
});

test("a suite without its own cleanup gets its sandbox repository deleted with the fixture token", async t => {
  const root = await temporary(t);
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  const requests: string[] = [];
  const request: typeof fetch = async (input, init) => {
    requests.push(`${init?.method} ${new URL(String(input)).pathname} ${new Headers(init?.headers).get("authorization")}`);
    return init?.method === "GET" ? Response.json({ id: 1, private: true }) : new Response(null, { status: 204 });
  };
  const suite: Suite = { name: "plain", scenarios: ["one"], variants: ["eval"], start: () => { throw new Error("unused"); } };
  const journal: Journal = { version: 2, suite: "plain", repo, id: 1, cleaned: false, resources: {} };
  await cleanupSuite(suite, journal, join(root, "resources.json"), { token: "agent", fixtureToken: "fixture", request });
  assert.deepEqual(requests, [`GET /repos/${repo} Bearer fixture`, `DELETE /repos/${repo} Bearer fixture`]);
  assert.equal(journal.cleaned, true);
});

test("first-version journals from earlier runs are read as events-suite journals", () => {
  const repo = "sandbox/run-00000000-0000-4000-8000-000000000001";
  assert.deepEqual(parseJournal({ version: 1, repo, id: 1, cleaned: false, deleting: true, subscriptions: ["sub_1"] }), {
    version: 2, suite: "events", repo, id: 1, cleaned: false, deleting: true, resources: { subscriptions: ["sub_1"] },
  });
  assert.throws(() => parseJournal({ version: 2, repo, cleaned: false, resources: {} }), /Invalid resource journal/);
});

test("transient subscription 503s are retried using Retry-After, while other writes and errors are not", async () => {
  const statuses: number[] = [];
  let calls = 0;
  const flaky = (async (input: string | URL | Request) => {
    calls++;
    const path = new URL(String(input)).pathname;
    if (path === "/event-subscriptions" && calls <= 2) {
      statuses.push(503);
      return new Response('{"message":"Service Unavailable"}', { status: 503, headers: { "retry-after": "0" } });
    }
    if (path.startsWith("/repos/")) { statuses.push(503); return new Response("{}", { status: 503 }); }
    statuses.push(201);
    return new Response(JSON.stringify({ id: "sub_ok", resource_url: "https://github.com/o/r/pull/1" }), { status: 201 });
  }) as typeof fetch;
  const github = new EventsGitHub("token", flaky, [0, 0, 0]);
  assert.equal((await github.subscribe("https://github.com/o/r/pull/1", ["com.github.pull_request.state.changed.v1"]))?.id, "sub_ok");
  assert.deepEqual(statuses, [503, 503, 201]);
  const before = calls;
  await assert.rejects(github.api("POST", "/repos/o/r/pulls/1/reviews", {}), /HTTP 503/);
  assert.equal(calls, before + 1, "a POST outside subscriptions must not be retried");
  const alwaysDown = new EventsGitHub("token", (async () => new Response("{}", { status: 503 })) as typeof fetch, [0, 0]);
  await assert.rejects(alwaysDown.api("POST", "/event-subscriptions", {}), /HTTP 503/);
});

test("a generic 404 from the subscription listing is retried, while a 404 for one subscription is final", async () => {
  let listings = 0;
  let deletes = 0;
  const flaky = (async (_input: string | URL | Request, init?: RequestInit) => {
    if (init?.method === "DELETE") {
      deletes++;
      return new Response('{"message":"Not Found"}', { status: 404 });
    }
    listings++;
    if (listings === 1) return new Response('{"message":"Not Found"}', { status: 404 });
    return new Response(JSON.stringify({ subscriptions: [] }), { status: 200 });
  }) as typeof fetch;
  const github = new EventsGitHub("token", flaky, [0, 0]);
  assert.deepEqual(await github.subscriptions(), []);
  assert.equal(listings, 2);
  await assert.rejects(github.api("DELETE", "/event-subscriptions/sub_gone"), /HTTP 404/);
  assert.equal(deletes, 1);
});

test("actual command observation preserves argv, stdin, stdout, stderr and nonzero exit status", { timeout: 10_000 }, async t => {
  const root = await temporary(t);
  const binary = join(root, "real");
  const wrapper = join(root, "gh");
  await writeFile(binary, `#!${process.execPath}
process.stderr.write(JSON.stringify(process.argv.slice(2)));
process.stdin.pipe(process.stdout);
process.stdin.on("end", () => { process.exitCode = 7; });
`, { mode: 0o700 });
  const calls: Call[] = [];
  const observer = new CommandObserver(calls, () => {});
  await observer.listen(join(root, ".sock"));
  t.after(() => observer.close());
  await writeGhWrapper(wrapper, binary, join(root, ".sock"));
  const args = ["pr", "a command with spaces", "'quoted'"];
  const child = spawn(wrapper, args, { env: isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "not-a-real-token") });
  let stdout = "";
  let stderr = "";
  child.stdout.on("data", chunk => { stdout += chunk; });
  child.stderr.on("data", chunk => { stderr += chunk; });
  child.stdin.end("the original stdin");
  const code = await new Promise<number | null>((resolve, reject) => {
    child.on("error", reject);
    child.on("close", resolve);
  });
  assert.equal(code, 7);
  assert.equal(stdout, "the original stdin");
  assert.equal(stderr, JSON.stringify(args));
  assert.deepEqual(calls[0]!.arguments, args);
  assert.equal(calls[0]!.kind, "gh");
  assert.ok(calls[0]?.kind === "gh");
  assert.equal(calls[0].output.filter(chunk => chunk.stream === "stdout").map(chunk => chunk.text).join(""), stdout);
  assert.equal(calls[0].output.filter(chunk => chunk.stream === "stderr").map(chunk => chunk.text).join(""), stderr);
  assert.equal(calls[0]!.success, false);
  assert.ok(calls[0]!.endedAt! >= calls[0]!.startedAt);
  assert.equal(observer.error, undefined);
});

test("observed output streams before EOF and retains split UTF-8 bytes", { timeout: 10_000 }, async t => {
  const root = await temporary(t);
  const binary = join(root, "real");
  const wrapper = join(root, "gh");
  await writeFile(binary, `#!${process.execPath}
process.stdout.write(Buffer.from([0xc3]));
setTimeout(() => process.stdout.write(Buffer.from([0xa9])), 20);
process.stdin.resume();
process.stdin.on("end", () => { process.stdout.write("\\n"); });
`, { mode: 0o700 });
  const calls: Call[] = [];
  const observer = new CommandObserver(calls, () => {});
  await observer.listen(join(root, ".sock"));
  t.after(() => observer.close());
  await writeGhWrapper(wrapper, binary, join(root, ".sock"));
  const child = spawn(wrapper, ["event", "listen"], {
    env: isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused"),
  });
  child.stdout.setEncoding("utf8");
  t.after(() => { if (child.exitCode === null) child.kill("SIGTERM"); });
  const completed = new Promise<number | null>((resolve, reject) => {
    child.on("error", reject);
    child.on("close", resolve);
  });
  let output = "";
  await new Promise<void>(resolve => {
    child.stdout.on("data", chunk => { output += chunk; if (output === "\u00e9") resolve(); });
  });
  assert.equal(child.exitCode, null, "the output must arrive while the real command is still waiting");
  child.stdin.end();
  assert.equal(await completed, 0);
  assert.equal(output, "\u00e9\n");
  assert.ok(calls[0]?.kind === "gh");
  assert.equal(calls[0].output.map(chunk => chunk.text).join(""), "\u00e9\n");
  assert.equal(observer.error, undefined);
});

test("closing the stream consumer stops the producer and records the last output without late observer messages",
  { timeout: 10_000 }, async t => {
    const { child, calls, observer, completed } = await observedProcess(t, `
setInterval(() => {
  process.stdout.write("event\\n");
  process.stderr.write("diagnostic\\n");
}, 5);
`);
    child.stderr.resume();
    child.stdout.once("data", () => child.stdout.destroy());
    assert.notEqual(await completed, 0);
    assert.equal(observer.error, undefined);
    assert.ok(calls[0]?.kind === "gh");
    assert.ok(calls[0].output.length > 0);
    assert.ok(calls[0].endedAt !== undefined);
  });

test("command output beyond 2 MiB is an explicit observation error rather than truncated success",
  { timeout: 10_000 }, async t => {
    const { child, observer, completed } = await observedProcess(t, `
process.stdout.write("x".repeat(3 * 1024 * 1024));
`);
    child.stdout.resume();
    child.stderr.resume();
    assert.notEqual(await completed, 0);
    assert.match(observer.error?.message ?? "", /output exceeded 2 MiB/);
  });

test("SDK evidence distinguishes outer tools from actual gh invocations", () => {
  const calls: Call[] = [];
  observe(calls, { type: "tool.execution_start", data: { toolCallId: "a", arguments: { command: "echo gh event listen" } } }, 1);
  observe(calls, { type: "tool.execution_complete", data: { toolCallId: "a", success: true } }, 2);
  assert.deepEqual(calls, [{
    id: "tool:a", kind: "tool", arguments: { command: "echo gh event listen" }, startedAt: 1, endedAt: 2, success: true,
  }]);
});

test("isolated environments omit host tokens, instructions, settings and unrelated skills", () => {
  const env = isolatedEnvironment("/tmp/owned", "/usr/bin:/bin", "model-only");
  assert.equal(env.COPILOT_GITHUB_TOKEN, "model-only");
  assert.equal(env.COPILOT_CLI, "1");
  for (const key of ["GH_TOKEN", "GITHUB_TOKEN", "GH_EVAL_ACTOR_TOKEN", "EVALUATE_USE_HOST_COPILOT_HOME", "NODE_OPTIONS"]) {
    assert.equal(env[key], undefined);
  }
  assert.equal(env.HOME, "/tmp/owned");
  assert.equal(env.GIT_CONFIG_NOSYSTEM, "1");
  assert.equal(contamination({ output: "/hidden/answer.txt" }, ["/hidden/"]), "/hidden/");
  assert.equal(contamination({ output: "ordinary gh output" }, ["/hidden/"]), undefined);
  assert.equal(redact("secret-value c2VjcmV0LXZhbHVl", ["secret-value"]), "[REDACTED] [REDACTED]");
});

test("stimulus variants are recorded and unknown variants are rejected", () => {
  assert.equal(stimulusVariant("eval", eventsSuite.variants), "eval");
  assert.equal(stimulusVariant("design-probe", eventsSuite.variants), "design-probe");
  assert.equal(stimulusVariant("raw-api", eventsSuite.variants), "raw-api");
  assert.throws(() => stimulusVariant("agent-hint", eventsSuite.variants), /Unsupported stimulus variant/);
  assert.throws(() => stimulusVariant("typo", eventsSuite.variants), /Unsupported stimulus variant/);
});

test("development runs can shorten the agent budget without changing checked-in definitions", () => {
  assert.equal(effectiveAgentBudget("2m", 300_000, 600_000), 120_000);
  assert.equal(effectiveAgentBudget(undefined, 300_000, 600_000), 300_000);
  assert.equal(effectiveAgentBudget(undefined, undefined, 600_000), 600_000);
  assert.throws(() => effectiveAgentBudget("0s", 300_000, 600_000), /positive duration/);
});

test("an unfetched merge commit is a task failure, not a broken git verification command", async t => {
  const root = await temporary(t);
  const env = { ...isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused"),
    GIT_AUTHOR_NAME: "Fixture", GIT_AUTHOR_EMAIL: "fixture@example.invalid",
    GIT_COMMITTER_NAME: "Fixture", GIT_COMMITTER_EMAIL: "fixture@example.invalid" };
  await git(root, env, "init", "--initial-branch=main");
  await git(root, env, "commit", "--allow-empty", "-m", "Initial");
  const sha = await git(root, env, "rev-parse", "HEAD");
  assert.equal(await containsCommit(root, env, sha), true);
  assert.equal(await containsCommit(root, env, "0".repeat(40)), false);
});

for (const [scenario, branch, hasPr, requestCount, greeting, authoredByAgent] of [
  ["notify-review", "main", true, 3, "Hello from the merged change\n", 0],
  ["pull-after-merge", "main", true, 3, "Hello from the merged change\n", 0],
  ["pull-after-close", "main", true, 3, "Hello from the merged change\n", 0],
  ["full-delegation", "improve-greeting", false, 2, "Hello, world\n", 0],
  ["review-loop", "main", true, 2, "Hello, world\n", 1],
] as const) {
  test(`fixture provisioning prepares real isolated Git history for ${scenario}`, async t => {
    const root = await temporary(t);
    const remote = join(root, "remote");
    const workDir = join(root, "work");
    const env = {
      ...isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused"),
      GIT_AUTHOR_NAME: "Fixture", GIT_AUTHOR_EMAIL: "fixture@example.invalid",
      GIT_COMMITTER_NAME: "Fixture", GIT_COMMITTER_EMAIL: "fixture@example.invalid",
      GIT_CONFIG_COUNT: "1",
      GIT_CONFIG_KEY_0: `url.${remote}/.insteadOf`,
      GIT_CONFIG_VALUE_0: "https://github.com/",
    };
    const requests: string[] = [];
    const github = new EventsGitHub("unused", async (input, init) => {
      const path = new URL(String(input)).pathname;
      requests.push(`${init?.method} ${path}`);
      const body = JSON.parse(String(init?.body));
      if (init?.method === "POST" && path === "/orgs/sandbox/repos") {
        await mkdir(join(remote, "sandbox"), { recursive: true });
        await exec("git", ["init", "--bare", join(remote, "sandbox", `${body.name}.git`)], { env });
        return Response.json({ id: 1, full_name: `sandbox/${body.name}`, private: true });
      }
      if (init?.method === "PATCH") return Response.json({});
      if (init?.method === "POST" && path.endsWith("/pulls")) {
        return Response.json({ ...fixture().pr, html_url: `https://github.com${path.replace("/repos/", "/").replace("/pulls", "/pull/1")}` });
      }
      throw new Error(`Unexpected setup request: ${init?.method} ${path}`);
    });
    let agentPulls = 0;
    const agent = new EventsGitHub("agent-author", async (input, init) => {
      const path = new URL(String(input)).pathname;
      assert.equal(init?.method, "POST");
      assert.match(path, /^\/repos\/sandbox\/run-[a-f0-9-]+\/pulls$/);
      agentPulls++;
      return Response.json({ ...fixture().pr, html_url: `https://github.com${path.replace("/repos/", "/").replace("/pulls", "/pull/1")}` });
    });
    const { fixture: created, journal } = await provision(
      github, "sandbox", scenario, workDir, env, join(root, "resources.json"), agent,
    );
    assert.equal(journal.id, 1);
    assert.equal(await git(workDir, env, "status", "--porcelain"), "");
    assert.notEqual(created.baseSha, created.initialHead);
    assert.equal(await git(workDir, env, "branch", "--show-current"), branch);
    assert.equal(created.pr !== undefined, hasPr);
    assert.equal(requests.length, requestCount);
    assert.equal(agentPulls, authoredByAgent, "approval-loop PRs must be authored by the agent, not the reviewer App");
    assert.equal(await git(workDir, env, "show", `${created.initialHead}:greeting.txt`), greeting.trim());
  });
}

test("entrypoints list cases without credentials", async t => {
  const root = await temporary(t);
  const env = isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused");
  const runner = fileURLToPath(new URL("../run", import.meta.url));
  const list = await exec(runner, ["--list"], { env });
  assert.match(list.stdout, /notify-review/);
  assert.match(list.stdout, /pull-after-close/);
  const launcher = fileURLToPath(new URL("../../run-evals", import.meta.url));
  const launcherEnv = { ...env, GH_EVAL_REPO_ROOT: fileURLToPath(new URL("../..", import.meta.url)) };
  for (const [args, expected] of [
    [["--list"], /notify-review/],
    [["--live", "--list"], /notify-review/],
    [["--skill", "off", "--list"], /full-delegation/],
    [["--live", "--gh", "/missing/gh", "--list"], /notify-review/],
    [["--live", "--help"], /Usage:/],
    [["--live", "-h"], /Usage:/],
  ] as const) {
    const listed = await exec(launcher, [...args], { env: launcherEnv });
    assert.match(listed.stdout, expected);
  }
  for (const args of [["--live", "--", "--list"], ["--live", "--model", "--list", "notify-review"]]) {
    await assert.rejects(exec(launcher, args, { env: launcherEnv }), /Set GH_EVAL_TOKEN/);
  }
});

test("live launches need no actor token, and an actor token becomes the default fixture identity", async t => {
  const root = await temporary(t);
  const env = isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused");
  const runner = join(root, "runner");
  await writeFile(runner, "#!/bin/sh\nprintf '%s|%s' \"${GH_EVAL_ACTOR_TOKEN:-}\" \"${GH_EVAL_FIXTURE_TOKEN:-}\"\n", { mode: 0o700 });
  const launcher = fileURLToPath(new URL("../../run-evals", import.meta.url));
  const base = { ...env, GH_EVAL_RUNNER: runner, GH_EVAL_TOKEN: "user", COPILOT_GITHUB_TOKEN: "model" };
  assert.equal((await exec(launcher, ["--live"], { env: base })).stdout, "|");
  assert.equal((await exec(launcher, ["--live"], { env: { ...base, GH_EVAL_ACTOR_TOKEN: "actor" } })).stdout, "actor|actor");
  await assert.rejects(exec(launcher, ["--live"], { env: { ...base, GH_EVAL_ACTOR_TOKEN: "two tokens" } }),
    /GH_EVAL_ACTOR_TOKEN must contain one non-empty token/);
});

test("the runner stages the gh skill from a Git revision, not from the working tree", async t => {
  const root = await temporary(t);
  const checkout = join(root, "checkout");
  const env = isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused");
  const record = join(root, "staged.txt");
  await mkdir(join(root, "tmp"), { recursive: true });
  await mkdir(join(checkout, "evals/node_modules/.bin"), { recursive: true });
  await mkdir(join(checkout, "evals/dist"));
  await mkdir(join(checkout, "evals/suites/events/notify-review"), { recursive: true });
  await mkdir(join(checkout, "skills/gh/references"), { recursive: true });
  await writeFile(join(checkout, "evals/run"), await readFile(fileURLToPath(new URL("../run", import.meta.url))), { mode: 0o700 });
  await writeFile(join(checkout, "evals/dist/executor-plugin.js"), "");
  await writeFile(join(checkout, "evals/dist/cleanup.js"), "");
  await writeFile(join(checkout, "evals/suites/events/notify-review/eval.yml"), "");
  // The fake vally records each staged skill file with its content.
  await writeFile(join(checkout, "evals/node_modules/.bin/vally"), [
    "#!/bin/sh",
    "while [ $# -gt 0 ]; do",
    "  if [ \"$1\" = --skill-dir ]; then (cd \"$2\" && find . -type f | LC_ALL=C sort | while read -r f; do printf '%s=%s\\n' \"$f\" \"$(cat \"$f\")\"; done) > \"$RECORD\"; fi",
    "  shift",
    "done",
  ].join("\n"), { mode: 0o700 });
  const binary = join(root, "gh");
  await writeFile(binary, "not executed", { mode: 0o700 });
  await writeFile(join(checkout, "skills/gh/SKILL.md"), "committed");
  await writeFile(join(checkout, "skills/gh/references/extra.md"), "reference");
  await git(checkout, env, "init", "-q");
  await git(checkout, env, "add", "skills");
  await git(checkout, env, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "skill");
  await writeFile(join(checkout, "skills/gh/SKILL.md"), "uncommitted");

  const run = (...args: string[]) => exec(join(checkout, "evals/run"), [
    "--live", "--gh", binary, "--model", "m", "--skill", "on", "--output", join(root, "out"), ...args, "notify-review",
  ], { env: { ...env, RECORD: record } });
  const commit = await git(checkout, env, "rev-parse", "HEAD");

  const directory = await run("--gh-skill-ref", "HEAD:skills/gh/");
  assert.equal(await readFile(record, "utf8"), "./gh/SKILL.md=committed\n./gh/references/extra.md=reference\n");
  assert.match(directory.stderr, new RegExp(`gh skill: ${commit}:skills/gh \\(from HEAD:skills/gh/\\)`));
  await run("--gh-skill-ref", "HEAD:skills/gh/SKILL.md");
  assert.equal(await readFile(record, "utf8"), "./gh/SKILL.md=committed\n");

  for (const [args, expected] of [
    [["--gh-skill", join(checkout, "skills/gh"), "--gh-skill-ref", "HEAD:skills/gh"], /either --gh-skill or --gh-skill-ref/],
    [["--gh-skill-ref", "HEAD"], /must be REV:PATH/],
    [["--gh-skill-ref", "missing:skills/gh"], /Unknown Git revision/],
    [["--gh-skill-ref", "HEAD:skills"], /must name a skill directory/],
    [["--gh-skill-ref", "HEAD:skills/gh/references/extra.md"], /must name a skill directory/],
  ] as const) {
    await assert.rejects(run(...args), expected);
  }
});

test("Git uses only the fixture credential, including when its home path contains spaces", async t => {
  const root = await temporary(t);
  const home = join(root, "home with spaces");
  await mkdir(home);
  const gh = join(root, "unused-gh");
  await writeFile(gh, "not executed");
  const env = await prepareHome(home, { gh, token: "fixture-token", modelToken: "separate-model-token" });
  const child = spawn("git", ["credential", "fill"], { env });
  let stdout = "";
  let stderr = "";
  child.stdout.on("data", chunk => { stdout += chunk; });
  child.stderr.on("data", chunk => { stderr += chunk; });
  child.stdin.end("protocol=https\nhost=github.com\n\n");
  const code = await new Promise<number | null>((resolve, reject) => {
    child.on("error", reject);
    child.on("close", resolve);
  });
  assert.equal(code, 0, stderr);
  assert.match(stdout, /password=fixture-token/);
  assert.doesNotMatch(stdout, /separate-model-token/);
  assert.equal(env.GH_TOKEN, undefined);
  assert.equal(env.GITHUB_TOKEN, undefined);
});

test("the scripted smoke home uses the supplied binary directly without the command observer", async t => {
  const root = await temporary(t);
  const gh = join(root, "candidate-gh");
  await writeFile(gh, "#!/bin/sh\nexit 0\n", { mode: 0o700 });
  const home = join(root, "smoke-home");
  await prepareHome(home, { gh, token: "fixture-token", modelToken: "unused", observe: false });
  assert.equal(await readFile(join(home, "bin/gh"), "utf8"), "#!/bin/sh\nexit 0\n");
  await assert.rejects(access(join(home, "bin/gh-real")), { code: "ENOENT" });
  await assert.rejects(access(join(home, ".commands.sock")), { code: "ENOENT" });
});

test("the runner rejects an output symlink into the checkout without creating directories", async t => {
  const root = await temporary(t);
  const checkout = fileURLToPath(new URL("../..", import.meta.url));
  await symlink(checkout, join(root, "checkout"));
  const parent = `must-not-create-${Date.now()}`;
  const binary = join(root, "gh");
  await writeFile(binary, "not executed", { mode: 0o700 });
  const env = isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused");
  await assert.rejects(exec(fileURLToPath(new URL("../run", import.meta.url)), [
    "--live", "--gh", binary, "--model", "unused", "--skill", "off",
    "--output", join(root, "checkout", parent, "out"), "notify-review",
  ], { env }), /outside the checkout/);
  await assert.rejects(access(join(checkout, parent)), /ENOENT/);
});

test("the runner refuses live work without --live, unknown scenarios, and a missing gh binary", async t => {
  const root = await temporary(t);
  const env = isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused");
  const run = fileURLToPath(new URL("../run", import.meta.url));
  const binary = join(root, "gh");
  await writeFile(binary, "not executed", { mode: 0o700 });
  await assert.rejects(exec(run, ["--gh", binary, "--model", "m", "notify-review"], { env }), /pass --live/);
  await assert.rejects(exec(run, ["--live", "--gh", binary, "--model", "m", "--skill", "off", "nope"], { env }),
    /Unknown scenario: nope/);
  await assert.rejects(exec(run, ["--live", "--gh", join(root, "missing"), "--model", "m", "notify-review"], { env }),
    /does not exist/);
  await assert.rejects(exec(run, ["--live", "--gh", binary, "--model", "m", "--skill", "on", "notify-review"], { env }),
    /--gh-skill must be a skill directory/);
  await assert.rejects(exec(run, [
    "--live", "--gh", binary, "--model", "m", "--skill", "off", "--variant", "typo", "notify-review",
  ], { env }), /--variant must be one of/);
});

for (const limit of [10, 100]) {
  test(`seeding fills a subscription limit of ${limit} on other PRs, closing the stale ones afterwards`, async t => {
    const root = await temporary(t);
    const remote = join(root, "remote");
    const workDir = join(root, "work");
    const env = {
      ...isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused"),
      GIT_AUTHOR_NAME: "Fixture", GIT_AUTHOR_EMAIL: "fixture@example.invalid",
      GIT_COMMITTER_NAME: "Fixture", GIT_COMMITTER_EMAIL: "fixture@example.invalid",
      GIT_CONFIG_COUNT: "1",
      GIT_CONFIG_KEY_0: `url.${remote}/.insteadOf`,
      GIT_CONFIG_VALUE_0: "https://github.com/",
    };
    await exec("git", ["init", "--bare", join(remote, "sandbox", "run.git")], { env });
    await mkdir(workDir);
    await git(workDir, env, "init", "--initial-branch=main");
    await git(workDir, env, "remote", "add", "origin", "https://github.com/sandbox/run.git");
    await git(workDir, env, "commit", "--allow-empty", "-m", "Initial");
    await git(workDir, env, "push", "--set-upstream", "origin", "main");
    const heads: string[] = [];
    const closed: number[] = [];
    const events: string[] = [];
    const github = new EventsGitHub("unused", async (input, init) => {
      const path = new URL(String(input)).pathname;
      const body = JSON.parse(String(init?.body));
      if (init?.method === "POST" && path === "/repos/sandbox/run/pulls") {
        heads.push(body.head);
        events.push(`pr:${heads.length + 1}`);
        return Response.json({ ...fixture().pr, number: heads.length + 1, html_url: `https://github.com/sandbox/run/pull/${heads.length + 1}` });
      }
      const close = /^\/repos\/sandbox\/run\/pulls\/(\d+)$/.exec(path);
      if (init?.method === "PATCH" && close && body.state === "closed") {
        closed.push(Number(close[1]));
        events.push(`close:${close[1]}`);
        return Response.json({});
      }
      throw new Error(`Unexpected setup request: ${init?.method} ${path}`);
    });
    const held = new Map<string, string>();
    const subscriber = new EventsGitHub("unused", async (input, init) => {
      assert.equal(`${init?.method} ${new URL(String(input)).pathname}`, "POST /event-subscriptions");
      const body = JSON.parse(String(init?.body));
      const key = `${body.resource_url} ${[...body.event_types].sort().join(",")}`;
      assert.ok(!held.has(key), "each seeded subscription must be distinct, or the API would reuse it");
      if (held.size >= limit) return Response.json({ message: "Subscription limit exceeded" }, { status: 422 });
      held.set(key, `sub_${held.size + 1}`);
      events.push("subscribe");
      return Response.json({ id: held.get(key), resource_url: body.resource_url }, { status: 201 });
    });
    const journalPath = join(root, "resources.json");
    const journal: Journal = { version: 2, suite: "events", repo: "sandbox/run", cleaned: false, resources: {} };

    const seeded = await seedSubscriptions(github, subscriber, "sandbox/run", workDir, env, journal, journalPath);

    assert.equal(seeded.stale.length + seeded.active.length, limit);
    assert.ok(seeded.active.length >= 3 && seeded.stale.length > seeded.active.length);
    assert.deepEqual(journal.resources.subscriptions, [...held.values()]);
    assert.deepEqual(JSON.parse(await readFile(journalPath, "utf8")).resources.subscriptions, journal.resources.subscriptions);
    assert.ok([...held.keys()].every(key => !key.startsWith("https://github.com/sandbox/run/pull/1 ")),
      "the target PR must not be seeded");
    const stale = heads.flatMap((head, index) => head.startsWith("abandoned-") ? [index + 2] : []);
    assert.deepEqual(closed, stale);
    assert.ok(heads.some(head => head.startsWith("parallel-")));
    assert.ok(events.lastIndexOf("subscribe") < events.findIndex(event => event.startsWith("close:")),
      "PRs are closed only after seeding");
    for (const head of heads) {
      assert.equal(await git(workDir, env, "ls-remote", "origin", `refs/heads/${head}`) !== "", true);
    }
    assert.equal(await git(workDir, env, "branch", "--show-current"), "main");
  });
}

test("seeding fails setup when the limit leaves no room for both stale and active subscriptions", async t => {
  const root = await temporary(t);
  const workDir = join(root, "work");
  await mkdir(workDir);
  const env = { ...isolatedEnvironment(root, dirname(process.execPath) + ":/usr/bin:/bin", "unused"),
    GIT_AUTHOR_NAME: "Fixture", GIT_AUTHOR_EMAIL: "fixture@example.invalid",
    GIT_COMMITTER_NAME: "Fixture", GIT_COMMITTER_EMAIL: "fixture@example.invalid" };
  await exec("git", ["init", "--bare", join(root, "remote.git")], { env });
  await git(workDir, env, "init", "--initial-branch=main");
  await git(workDir, env, "remote", "add", "origin", join(root, "remote.git"));
  await git(workDir, env, "commit", "--allow-empty", "-m", "Initial");
  let pulls = 0;
  const github = new EventsGitHub("unused", async () => Response.json({ ...fixture().pr, number: ++pulls + 1,
    html_url: `https://github.com/sandbox/run/pull/${pulls + 1}` }));
  let created = 0;
  const subscriber = new EventsGitHub("unused", async (_input, init) => created++
    ? Response.json({ message: "Subscription limit exceeded" }, { status: 422 })
    : Response.json({ id: "sub_1", resource_url: JSON.parse(String(init?.body)).resource_url }, { status: 201 }));
  const journal: Journal = { version: 2, suite: "events", repo: "sandbox/run", cleaned: false, resources: {} };
  await assert.rejects(seedSubscriptions(github, subscriber, "sandbox/run", workDir, env, journal, join(root, "j.json")),
    /too low to seed both stale and active/);
  assert.deepEqual(journal.resources.subscriptions, ["sub_1"], "the created subscription stays journaled for cleanup");
});
