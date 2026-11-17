import assert from "node:assert/strict";
import { test } from "node:test";
import { computeMetrics, type Trajectory } from "@microsoft/vally";
import { Actor } from "./suites/events/actor.js";
import { ActorDriver } from "./core/actor.js";
import { EventsGitHub } from "./suites/events/github.js";
import { afterSuccessfulTool } from "./core/session.js";
import { proposedGreeting, reviewedGreeting, type Fixture, type Scenario } from "./suites/events/domain.js";
import { type GhCall } from "./core/calls.js";
import { finalLoopGreeting } from "./suites/events/scenarios/review-loop.js";
import { EventTaskGrader } from "./suites/events/grading/task.js";
import { EventRouteGrader } from "./suites/events/grading/route.js";
import type { Evidence } from "./suites/events/grading/evidence.js";

const prUrl = "https://github.com/sandbox/run/pull/1";

function rig(scenario: Scenario = "review-loop") {
  let head = "initial";
  let greeting: string = proposedGreeting;
  const reviews: Array<{ id: number; state: string; commit_id: string; body: string }> = [];
  const pull = () => ({
    number: 1, html_url: prUrl, state: "open", merged: false, mergeable: true, merge_commit_sha: null,
    head: { sha: head, ref: "improve-greeting" }, base: { ref: "main" },
  });
  const fixture: Fixture = { scenario, repo: "sandbox/run", workDir: "/unused", baseSha: "base", initialHead: head, pr: pull() };
  const github = new EventsGitHub("test-only", async (input, init) => {
    const url = new URL(String(input));
    if (init?.method === "GET" && url.pathname === "/repos/sandbox/run/pulls/1") return Response.json(pull());
    if (init?.method === "GET" && url.pathname === "/repos/sandbox/run/contents/greeting.txt") {
      assert.equal(url.searchParams.get("ref"), head);
      return Response.json({ encoding: "base64", content: Buffer.from(greeting).toString("base64") });
    }
    if (init?.method === "POST" && url.pathname === "/repos/sandbox/run/pulls/1/reviews") {
      const body: Record<string, unknown> = JSON.parse(String(init.body));
      assert.ok(typeof body.commit_id === "string" && typeof body.event === "string" && typeof body.body === "string");
      const review = {
        id: 100 + reviews.length, commit_id: body.commit_id, body: body.body,
        state: body.event === "APPROVE" ? "APPROVED" : body.event === "COMMENT" ? "COMMENTED" : "CHANGES_REQUESTED",
      };
      reviews.push(review);
      return Response.json(review);
    }
    throw new Error(`Unexpected request: ${init?.method} ${url}`);
  });
  return {
    fixture, github, reviews, pull,
    get greeting() { return greeting; },
    push(sha: string, text: string) { head = sha; greeting = text; },
  };
}

function wait(id: string, at: number): GhCall {
  return {
    id, kind: "gh", arguments: ["event", "listen", "--resource", prUrl, "--event", "review", "--count", "1"],
    startedAt: at, output: [],
  };
}

const trajectory: Trajectory = {
  id: "loop-control", stimulus: { name: "review-loop", prompt: "Address feedback until approved; do not merge." },
  events: [], output: "Both fixes are pushed and approved.", workDir: "/unused",
  metadata: { executor: "gh-agent", skillsLoaded: [], model: "control", sessionID: "control" },
  metrics: { ...computeMetrics([]), wallTimeMs: 100 },
};

async function completedLoop(): Promise<Evidence> {
  const remote = rig();
  const actor = new Actor(remote.github, remote.github, remote.fixture, 100, 0);
  const first = wait("first", 5_000);
  await actor.tick(2_000, undefined, "post-tool");
  remote.push("fix-one", reviewedGreeting);
  await actor.tick(20_000);
  const second = wait("second", 25_000);
  await actor.tick(30_000, second);
  remote.push("fix-two", finalLoopGreeting);
  await actor.tick(40_000, undefined, "post-tool");
  const approval = wait("approval", 50_000);
  for (const [call, review, at] of [
    [first, remote.reviews[0]!, 12_000], [second, remote.reviews[1]!, 32_000], [approval, remote.reviews[2]!, 52_000],
  ] as const) {
    call.endedAt = at + 1;
    call.success = true;
    call.exitCode = 0;
    call.output = [{ stream: "stdout", at, text: JSON.stringify({
      id: `event-${review.id}`, type: "com.github.pull_request.review.submitted.v1",
      data: { api_url: `https://api.github.com/repos/sandbox/run/pulls/1/reviews/${review.id}` },
    }) + "\n" }];
  }
  return {
    version: 3, scenario: "review-loop", variant: "eval", skill: true,
    calls: [first, second, approval],
    snapshot: {
      observedUntil: 100_000, actions: actor.actions, pr: remote.pull(), reviews: remote.reviews,
      greeting: remote.greeting, localGreeting: remote.greeting, currentBranch: "improve-greeting", mainContainsMerge: false,
    },
  };
}

test("the first review arrives after the first tool, then the actor requires a second wait and posts approval during work", async () => {
  const remote = rig();
  const actor = new Actor(remote.github, remote.github, remote.fixture, 100, 0);
  await actor.tick(10, wait("first", 5));
  assert.equal(remote.reviews.length, 0, "timer and wait ticks must not deliver the startup-gap review");
  await actor.tick(11, undefined, "post-tool");
  assert.equal(remote.reviews[0]?.state, "CHANGES_REQUESTED");
  assert.equal(actor.actions[0]?.trigger, "after-tool");
  remote.push("fix-one", "Still wrong\n");
  await actor.tick(20);
  assert.equal(actor.actions.length, 1, "an unverified file must not unlock another round");
  remote.push("fix-one", reviewedGreeting);
  await actor.tick(25);
  assert.equal(remote.reviews.length, 1, "the first handled review remains present while waiting for the next round");
  await actor.tick(35, wait("second", 30));
  assert.equal(remote.reviews[1]?.state, "CHANGES_REQUESTED");
  assert.match(remote.reviews[1]!.body, /replace world with GitHub/);
  await actor.tick(40);
  assert.equal(remote.reviews.length, 2, "approval must require a distinct second pushed fix");
  remote.push("fix-two", finalLoopGreeting);
  await actor.tick(45);
  assert.equal(remote.reviews.length, 2, "an after-fix approval must not be delivered by the timer alone");
  const order: string[] = [];
  const driver = new ActorDriver(actor, async () => { order.push("review-posted"); }, () => 50);
  const original = { additionalContext: "existing hook context" };
  const config = afterSuccessfulTool({
    hooks: { onPostToolUse: () => { order.push("tool-succeeded"); return original; } },
  }, () => driver.tick(undefined, "post-tool"));
  const result = await config.hooks!.onPostToolUse!({
    sessionId: "control", timestamp: new Date(50), workingDirectory: "/unused",
    toolName: "bash", toolArgs: { command: "git push" },
    toolResult: { resultType: "success", textResultForLlm: "Pushed" },
  }, { sessionId: "control" });
  order.push("agent-resumes");
  assert.deepEqual(order, ["tool-succeeded", "review-posted", "agent-resumes"]);
  assert.equal(result, original, "the hook must not add feedback or instructions to the tool result");
  assert.equal(remote.reviews[2]?.state, "APPROVED");
  assert.equal(remote.reviews[2]?.commit_id, "fix-two");
  assert.equal(actor.actions.at(-1)?.trigger, "after-fix");
  assert.equal(actor.done, true);
  await driver.close();
});

test("a persistent listener started before the first review can deliver the next round", async () => {
  const remote = rig();
  const actor = new Actor(remote.github, remote.github, remote.fixture, 100, 0);
  const follow: GhCall = { ...wait("follow", 1), arguments: ["event", "listen", "--resource", prUrl, "--follow"] };
  await actor.tick(2, undefined, "post-tool");
  remote.push("fix-one", reviewedGreeting);
  await actor.tick(10, wait("one-shot", 1));
  assert.equal(remote.reviews.length, 1, "a one-shot listener from the previous round must not deliver this one");
  await actor.tick(11, follow);
  assert.equal(remote.reviews.length, 2);
  assert.equal(actor.actions.at(-1)?.trigger, "wait");
  assert.equal(actor.actions.at(-1)?.callId, "follow");
});

test("an attached listener takes the after-fix approval once the fix is verified", async () => {
  const remote = rig();
  const actor = new Actor(remote.github, remote.github, remote.fixture, 100, 0);
  await actor.tick(2, undefined, "post-tool");
  remote.push("fix-one", reviewedGreeting);
  await actor.tick(10, wait("second", 9));
  await actor.tick(20, wait("approval", 19));
  assert.equal(remote.reviews.length, 2, "the listener must not take the approval before the fix is pushed");
  remote.push("fix-two", finalLoopGreeting);
  await actor.tick(25, wait("approval", 19));
  assert.equal(remote.reviews[2]?.state, "APPROVED");
  assert.equal(actor.actions.at(-1)?.trigger, "after-fix");
  assert.equal(actor.actions.at(-1)?.callId, "approval");
});

test("concurrent timer and post-tool ticks submit a review only once", async () => {
  const remote = rig("notify-review");
  const actor = new Actor(remote.github, remote.github, remote.fixture, 100, 0);
  const driver = new ActorDriver(actor, async () => {}, () => 10);
  await Promise.all([driver.tick(wait("first", 5)), driver.tick(wait("first", 5))]);
  await driver.close();
  assert.equal(remote.reviews.length, 1);
  assert.equal(actor.actions.length, 1);
});

test("actor callback failures are propagated and drained rather than becoming an agent failure", async () => {
  const remote = rig("notify-review");
  const driver = new ActorDriver(new Actor(remote.github, remote.github, remote.fixture, 100, 0),
    async () => { throw new Error("failed to save actor evidence"); }, () => 10);
  await assert.rejects(driver.tick(wait("first", 5)), /failed to save actor evidence/);
  await assert.rejects(driver.close(), /failed to save actor evidence/);
});

test("a fallback approval is graded on the same evidence as an after-fix one", async () => {
  const facts = await completedLoop();
  facts.snapshot.actions.at(-1)!.trigger = "fallback";
  const route = await new EventRouteGrader(async () => facts).grade({ trajectory });
  assert.equal(route.passed, true);
  facts.calls.splice(2);
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, false);
});

test("two review rounds followed by already-arrived approval pass without prescribing cursor syntax", async () => {
  const facts = await completedLoop();
  const load = async () => facts;
  assert.equal((await new EventTaskGrader(load).grade({ trajectory })).passed, true);
  const route = await new EventRouteGrader(load).grade({ trajectory });
  assert.equal(route.passed, true);
  assert.equal(facts.snapshot.actions.at(-1)?.trigger, "after-fix");
  assert.ok(facts.calls[2]!.startedAt > facts.snapshot.actions.at(-1)!.at);
  assert.equal(route.metadata?.dedicatedWaits, 3);
  const observations = route.metadata?.observations;
  assert.ok(Array.isArray(observations));
  assert.deepEqual(observations.map(observation => observation.recovered), [true, false, true]);
});

test("a review posted outside any listener is recovered by any status read that shows it", async () => {
  const facts = await completedLoop();
  const firstReview = facts.snapshot.reviews[0]!;
  const stream: GhCall = {
    ...wait("stream", 5_000),
    arguments: ["event", "listen", "--resource", prUrl, "--event", "review,state"],
    output: [],
  };
  const snapshot: GhCall = {
    ...wait("snapshot", 6_000),
    arguments: ["pr", "view", "1", "--json", "reviews,reviewDecision,headRefOid"],
    endedAt: 7_000, success: true, exitCode: 0,
    output: [{ stream: "stdout", at: 7_000, text: JSON.stringify({
      reviewDecision: "CHANGES_REQUESTED",
      reviews: [{
        id: "PRR_graphql_node_id", state: firstReview.state, body: firstReview.body,
        commit: { oid: firstReview.commit_id },
      }],
    }) }],
  };
  facts.calls = [stream, snapshot, ...facts.calls.slice(1)];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, true);
  const latest = { ...snapshot, arguments: ["pr", "view", "1", "--json", "latestReviews"] };
  facts.calls = [stream, latest, ...facts.calls.slice(2)];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, true,
    "latestReviews shows the review too");
  facts.calls = [stream, snapshot, ...facts.calls.slice(2)];
  snapshot.startedAt = 5_001;
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, true,
    "a snapshot launched in parallel with the listener still shows the review");
  snapshot.startedAt = 1_000;
  facts.calls = [snapshot, stream, ...facts.calls.slice(1)];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, true,
    "a read before listening that already shows the review leaves nothing missed");
  snapshot.output[0]!.text = JSON.stringify({ reviewDecision: "", reviews: [] });
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, false,
    "a read taken before the review landed, with no later read or event showing it, misses the review");
});

test("a review posted outside any listener is recovered from rendered output that shows its body", async () => {
  const facts = await completedLoop();
  const firstReview = facts.snapshot.reviews[0]!;
  const stream: GhCall = {
    ...wait("stream", 5_000),
    arguments: ["event", "listen", "--resource", prUrl, "--event", "review,state"],
    output: [],
  };
  const body = String(firstReview.body).split(" ");
  const comments: GhCall = {
    ...wait("comments", 6_000),
    arguments: ["pr", "view", "1", "--comments"],
    endedAt: 7_000, success: true, exitCode: 0,
    output: [
      { stream: "stdout", at: 6_500, text: `author:\treviewer\nstate:\tCHANGES_REQUESTED\n--\n${body.slice(0, 4).join(" ")}` },
      { stream: "stdout", at: 7_000, text: `\n${body.slice(4).join(" ")}\n--\n` },
    ],
  };
  facts.calls = [stream, comments, ...facts.calls.slice(1)];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, true);
  comments.output = [{ stream: "stdout", at: 7_000, text: "no reviews yet\n" }];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, false,
    "rendered output without the review body shows nothing");
});

test("returning the same first review on the second wait cannot pass the loop route", async () => {
  const facts = await completedLoop();
  const second = facts.calls[1] as GhCall;
  second.output[0]!.text = second.output[0]!.text.replace("/reviews/101", "/reviews/100");
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, false);
});

test("a persistent stream can carry all loop rounds and approval while work happens", async () => {
  const facts = await completedLoop();
  const stream: GhCall = {
    ...wait("stream", 5_000), arguments: ["event", "listen", "--subscription", "sub_loop"],
    output: [12_000, 32_000, 42_000].map((at, index) => ({
      stream: "stdout", at, text: JSON.stringify({
        id: `event-${index}`, type: "com.github.pull_request.review.submitted.v1",
        data: { api_url: `https://api.github.com/repos/sandbox/run/pulls/1/reviews/${facts.snapshot.reviews[index]!.id}` },
      }) + "\n",
    })),
  };
  facts.calls = [stream];
  for (const action of facts.snapshot.actions.filter(action => action.trigger === "wait")) action.callId = stream.id;
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, true);
});

test("one reshaped event cannot stand in for several review rounds", async () => {
  const facts = await completedLoop();
  const stream = (...times: number[]): GhCall => ({
    ...wait("stream", 5_000),
    arguments: ["event", "listen", "--resource", prUrl, "--event", "review", "--follow", "--jq", ".id"],
    output: times.map(at => ({ stream: "stdout", at, text: `event-${at}\n` })),
  });
  for (const action of facts.snapshot.actions.filter(action => action.trigger === "wait")) action.callId = "stream";
  facts.calls = [stream(12_000, 32_000, 42_000)];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, true);
  facts.calls = [stream(42_000)];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, false,
    "only the approval printed, so the earlier rounds were missed");
  facts.calls = [stream(32_000, 42_000)];
  assert.equal((await new EventRouteGrader(async () => facts).grade({ trajectory })).passed, false,
    "the first round printed nothing");
});

test("a missed second fix or approval on an older head cannot satisfy the review loop", async () => {
  const facts = await completedLoop();
  facts.snapshot.actions = facts.snapshot.actions.filter(action => action.kind !== "fix" || action.head !== "fix-two");
  assert.equal((await new EventTaskGrader(async () => facts).grade({ trajectory })).passed, false);
  const stale = await completedLoop();
  stale.snapshot.reviews.at(-1)!.commit_id = "fix-one";
  assert.equal((await new EventTaskGrader(async () => stale).grade({ trajectory })).passed, false);
});

test("reusing the first review ID as a second review cannot satisfy the loop task", async () => {
  const facts = await completedLoop();
  const reviews = facts.snapshot.actions.filter(action => action.kind === "review");
  reviews[1]!.reviewId = reviews[0]!.reviewId;
  assert.equal((await new EventTaskGrader(async () => facts).grade({ trajectory })).passed, false);
});

test("additional pushes within a review round do not masquerade as or invalidate a later round", async () => {
  const facts = await completedLoop();
  facts.snapshot.actions.splice(1, 0, {
    kind: "fix", at: 15_000, pr: 1, head: "intermediate-first-fix", trigger: "verification",
  });
  assert.equal((await new EventTaskGrader(async () => facts).grade({ trajectory })).passed, true);
});
