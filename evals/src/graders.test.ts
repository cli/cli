import assert from "node:assert/strict";
import { test } from "node:test";
import { computeMetrics, createGraderRegistry, gradeTrajectory, loadEvalSpec, type Trajectory } from "@microsoft/vally";
import { execFile } from "node:child_process";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { registerEventGraders } from "./suites/events/grading/index.js";
import { evidenceFile, evidenceLoader, readTrialEvidence, type Evidence } from "./suites/events/grading/evidence.js";
import { NoPollingGrader } from "./suites/events/grading/polling.js";
import { EventRouteGrader } from "./suites/events/grading/route.js";
import { EventTaskGrader } from "./suites/events/grading/task.js";
import { mergedGreeting, originalGreeting, reviewedGreeting, scenarioNames } from "./suites/events/domain.js";
import { type GhCall } from "./core/calls.js";
import { prUrlPlaceholder } from "./suites/events/scenarios.js";
import { definitionPath, parseExecutorConfig, resolvePrompt } from "./core/suite.js";
import { suites } from "./suites/index.js";
import { SemanticReviewGrader } from "./graders/review.js";

const exec = promisify(execFile);
const fixed = (context: Evidence) => async () => context;
const prUrl = "https://github.com/sandbox/run/pull/1";
const trajectory: Trajectory = {
  id: "control", stimulus: { name: "notify-review", prompt: `Let me know when someone reviews my PR: ${prUrl}` },
  events: [], output: "A review has arrived.", workDir: "/unused",
  metadata: { executor: "gh-agent", skillsLoaded: [], model: "control", sessionID: "control" },
  metrics: { ...computeMetrics([]), wallTimeMs: 20 },
};

function wait(): GhCall {
  return {
    id: "gh:wait", kind: "gh", arguments: ["event", "listen", "--resource", prUrl, "--event", "review", "--count", "1"],
    startedAt: 1, endedAt: 20, success: true, exitCode: 0, output: [stream()],
  };
}

function evidence(): Evidence {
  return {
    version: 3, scenario: "notify-review", variant: "eval", skill: false, calls: [wait()],
    snapshot: {
      observedUntil: 20,
      actions: [{ kind: "review", at: 10, pr: 1, head: "head", trigger: "wait", callId: "gh:wait", reviewId: 7 }],
      reviews: [{ id: 7, state: "COMMENTED", commit_id: "head" }],
      currentBranch: "main", mainContainsMerge: false, localGreeting: originalGreeting,
      pr: {
        number: 1, html_url: prUrl, state: "open", merged: false, mergeable: true,
        merge_commit_sha: null, head: { sha: "head", ref: "topic" }, base: { ref: "main" },
      },
    },
  };
}

function stream(at = 15, id = "event-review", reviewId = 7): GhCall["output"][number] {
  return { at, stream: "stdout", text: JSON.stringify({
    id, type: "com.github.pull_request.review.submitted.v1",
    data: { api_url: `https://api.github.com/repos/sandbox/run/pulls/1/reviews/${reviewId}` },
  }) + "\n" };
}

function read(id: string, at: number): GhCall {
  return {
    id, kind: "gh", arguments: ["pr", "view", "1", "--json", "reviews"],
    startedAt: at, endedAt: at + 0.1, success: true, exitCode: 0, output: [],
  };
}

test("all mechanical graders pass a real wait result without a model", async () => {
  const context = evidence();
  const registry = createGraderRegistry();
  const checks = registerEventGraders(registry, fixed(context));
  assert.ok(checks.every(check => !check.metadata.behavior.requiresLlmClient && check.metadata.costProfile === "free"));
  const grade = await gradeTrajectory(trajectory, [{ type: "event-task" }, { type: "event-route" }, { type: "no-polling" }], { registry });
  assert.equal(grade.passed, true);
});

for (const [label, change] of [
  ["the event preceded command invocation", (context: Evidence) => { context.calls[0]!.startedAt = 11; }],
  ["the command had already returned", (context: Evidence) => { context.calls[0]!.endedAt = 9; }],
  ["the actor used its fallback", (context: Evidence) => { context.snapshot.actions[0]!.trigger = "fallback"; }],
  ["an outer shell call is mistaken for a gh process", (context: Evidence) => { context.calls[0]!.kind = "tool"; }],
  ["the wait printed no evidence", (context: Evidence) => { (context.calls[0] as GhCall).output = []; }],
  ["the wait observed an old review", (context: Evidence) => {
    (context.calls[0] as GhCall).output[0]!.text = stream(15, "event-old", 6).text;
  }],
  ["the result was printed before the actor", (context: Evidence) => { (context.calls[0] as GhCall).output[0]!.at = 9; }],
  ["the result refers to another PR", (context: Evidence) => {
    (context.calls[0] as GhCall).output[0]!.text = (context.calls[0] as GhCall).output[0]!.text
      .replace("/sandbox/run/", "/sandbox/other/");
  }],
  ["the output timestamp is after command completion", (context: Evidence) => {
    context.snapshot.observedUntil = 30;
    (context.calls[0] as GhCall).output[0]!.at = 21;
  }],
  ["a direct API stream was used", (context: Evidence) => { context.calls[0]!.arguments = ["api", "/event-subscriptions/sub_x/events"]; }],
  ["a help command merely described the right syntax", (context: Evidence) => { (context.calls[0] as GhCall).arguments.push("--help"); }],
] as const) {
  test(`route negative control: ${label}`, async () => {
    const context = evidence();
    change(context);
    assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, false);
  });
}

test("repeated status reads are polling; skill-off discovery is allowed only before the dedicated wait", async () => {
  const context = evidence();
  context.calls.unshift(read("read-1", -2), read("read-2", -1));
  const polling = () => new NoPollingGrader(fixed(context)).grade({ trajectory });
  assert.equal((await polling()).passed, true);
  context.skill = true;
  assert.equal((await polling()).passed, false);
  context.skill = false;
  context.calls.push(read("read-3", 16), read("read-4", 17));
  assert.equal((await polling()).passed, false);
});

test("polling is scored separately: observing the event does not excuse polling, and polling does not fail observation", async () => {
  const context = evidence();
  context.skill = true;
  context.calls.unshift(read("read-1", -2), read("read-2", -1));
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, true);
  const result = await new NoPollingGrader(fixed(context)).grade({ trajectory });
  assert.equal(result.passed, false);
  assert.equal(result.metadata?.pollingReads, 1);
  assert.equal(result.metadata?.statusReads, 2);
});

test("skill-off polling never followed by a dedicated wait is not excused as discovery", async () => {
  const context = evidence();
  context.calls = [read("read-1", 1), read("read-2", 2), read("read-3", 3)];
  assert.equal((await new NoPollingGrader(fixed(context)).grade({ trajectory })).passed, false);
});

test("a blocked dedicated wait with no polling passes no-polling even when the task and route fail", async () => {
  const context = evidence();
  context.skill = true;
  (context.calls[0] as GhCall).output = [];
  context.snapshot.reviews = [];
  assert.equal((await new NoPollingGrader(fixed(context)).grade({ trajectory })).passed, true);
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, false);
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, false);
});

test("initial inspection, concurrent initial inspection, and a read after an event are not polling", async () => {
  const context = evidence();
  context.skill = true;
  context.calls.unshift(read("initial", 5));
  context.calls.push(read("after-event", 16));
  assert.equal((await new NoPollingGrader(fixed(context)).grade({ trajectory })).passed, true);
});

test("repeating a read once a listener is running and again after it exits is not polling", async () => {
  const context = evidence();
  context.skill = true;
  context.snapshot.actions[0]!.at = 25;
  context.calls.unshift(read("before-listener", 0));
  context.calls.push(read("once-running", 2), read("after-exit", 21));
  assert.equal((await new NoPollingGrader(fixed(context)).grade({ trajectory })).passed, true);
  context.calls.push(read("repeat", 22));
  assert.equal((await new NoPollingGrader(fixed(context)).grade({ trajectory })).passed, false);
});

test("a review delivered by another listener running alongside the actor's one is observed", async () => {
  const context = evidence();
  context.calls.unshift({
    ...wait(), id: "gh:state", arguments: ["event", "listen", "--resource", prUrl, "--event", "state"], output: [],
  });
  context.snapshot.actions[0]!.callId = "gh:state";
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, true);
  (context.calls[1] as GhCall).startedAt = 11;
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, false,
    "a listener started after the review cannot have delivered it");
});

test("repeating a read after the listener reattached its stream is not polling", async () => {
  const context = evidence();
  context.skill = true;
  context.snapshot.actions[0]!.at = 25;
  (context.calls[0] as GhCall).output = [
    { stream: "stderr", at: 2, text: "Listening for events.\n" },
    { stream: "stderr", at: 6, text: "Event stream closed, reconnecting.\n" },
    { stream: "stderr", at: 8, text: "Listening for events.\n" },
  ];
  context.calls.push(read("once-running", 3), read("after-reattach", 9));
  assert.equal((await new NoPollingGrader(fixed(context)).grade({ trajectory })).passed, true);
  context.calls.push(read("repeat", 10));
  assert.equal((await new NoPollingGrader(fixed(context)).grade({ trajectory })).passed, false);
});

for (const args of [
  ["event", "listen", "--resource", prUrl, "--event", "review"],
  ["event", "listen", "--subscription", "sub_fixture"],
]) {
  test(`${args.slice(0, 3).join(" ")} can be stopped after delivering the event`, async () => {
    const context = evidence();
    context.calls = [{ ...wait(), arguments: args, success: false, exitCode: null, output: [stream()] }];
    assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, true);
    const call = context.calls[0] as GhCall;
    delete call.endedAt;
    assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, true);
    call.output[0]!.text = call.output[0]!.text.replace("/sandbox/run/", "/sandbox/other/");
    assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, false);
  });
}

test("filtered listener output counts only when bound to the fixture PR and the change's event type", async () => {
  const filtered = (...args: string[]) => {
    const context = evidence();
    context.calls = [{ ...wait(), arguments: ["event", "listen", ...args],
      output: [{ at: 15, stream: "stdout", text: "https://api.github.com/repos/sandbox/run/pulls/1/reviews/7\n" }] }];
    return new EventRouteGrader(fixed(context)).grade({ trajectory });
  };
  assert.equal((await filtered("--resource", prUrl, "--event", "review", "--jq", ".data.api_url")).passed, true);
  assert.equal((await filtered("--resource", prUrl, "--event", "state,review", "-t", "{{.id}}")).passed, true);
  assert.equal((await filtered("--resource", prUrl, "--event", "review")).passed, false);
  assert.equal((await filtered("--resource", prUrl, "--event", "state", "--jq", ".id")).passed, false);
  assert.equal((await filtered("--subscription", "sub_x", "--jq", ".id")).passed, false);
});

test("a listener bound by a REST API pull request URL counts as bound to the fixture PR", async () => {
  const route = (resource: string, ...extra: string[]) => {
    const context = evidence();
    context.snapshot.pr!.base = { ref: "main", repo: { id: 42 } };
    const output = extra.length ? [{ at: 15, stream: "stdout" as const, text: "7\n" }] : [stream()];
    context.calls = [{ ...wait(), arguments: ["event", "listen", "--resource", resource, "--event", "review", ...extra],
      output }];
    return new EventRouteGrader(fixed(context)).grade({ trajectory });
  };
  assert.equal((await route("https://api.github.com/repos/sandbox/run/pulls/1")).passed, true);
  assert.equal((await route("https://api.github.com/repositories/42/pulls/1")).passed, true);
  assert.equal((await route("https://api.github.com/repos/sandbox/run/pulls/1", "--jq", ".id")).passed, true);
  assert.equal((await route("https://api.github.com/repos/sandbox/run/pulls/2")).passed, false);
  assert.equal((await route("https://api.github.com/repositories/43/pulls/1", "--jq", ".id")).passed, false);
});

test("a notification claim without the scripted review cannot pass the state checks", async () => {
  const context = evidence();
  context.snapshot.reviews = [];
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, false);
});

test("pull-after-merge requires the local merged file and branch, not just remote state", async () => {
  const context = evidence();
  context.scenario = "pull-after-merge";
  context.snapshot.pr!.merged = true;
  context.snapshot.actions[0]!.kind = "merge";
  context.snapshot.mainContainsMerge = true;
  context.snapshot.localGreeting = mergedGreeting;
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, true);
  context.snapshot.mainContainsMerge = false;
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, false);
});

test("an unmerged close is observed by the state event and leaves main unchanged", async () => {
  const context = evidence();
  context.scenario = "pull-after-close";
  context.snapshot.pr!.state = "closed";
  context.snapshot.actions[0]!.kind = "close";
  (context.calls[0] as GhCall).output = [{ at: 15, stream: "stdout", text: JSON.stringify({
    id: "event-close", type: "com.github.pull_request.state.changed.v1",
    data: { api_url: "https://api.github.com/repos/sandbox/run/pulls/1" },
  }) + "\n" }];
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, true);
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, true);
  context.snapshot.mainContainsMerge = true;
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, false);
});

test("full delegation verifies the pushed fix and approval of the merged head", async () => {
  const context = evidence();
  context.scenario = "full-delegation";
  context.snapshot.pr!.merged = true;
  context.snapshot.pr!.head.sha = "fixed-head";
  context.snapshot.greeting = reviewedGreeting;
  context.snapshot.actions.push(
    { kind: "fix", at: 12, pr: 1, head: "fixed-head", trigger: "verification" },
    { kind: "approval", at: 15, pr: 1, head: "fixed-head", trigger: "wait", callId: "gh:wait", reviewId: 8 },
  );
  context.snapshot.reviews.push({ id: 8, state: "APPROVED", commit_id: "fixed-head" });
  context.calls = [{ ...wait(), output: [stream(11), stream(17, "event-approval", 8)] }];
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, true);
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, true);
  context.snapshot.reviews[1]!.commit_id = "old-head";
  assert.equal((await new EventTaskGrader(fixed(context)).grade({ trajectory })).passed, false);
  (context.calls[0] as GhCall).output.pop();
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, false);
});

test("a prose semantic failure fails the run even when both deterministic checks pass", async () => {
  const context = evidence();
  const registry = createGraderRegistry();
  const checks = registerEventGraders(registry, fixed(context));
  registry.register(new SemanticReviewGrader(async request => {
    assert.match(request.prompt, /whole trajectory/);
    assert.match(request.prompt, /blind spots/);
    assert.match(request.prompt, /recorded|Recorded/);
    assert.match(request.prompt, /"passed":true/);
    assert.doesNotMatch(request.prompt, /submit_verdict|dedicatedWaitCallIds/);
    return "The agent observed the review but only promised to notify later. This escaped the state checks.\nVerdict: FAIL";
  }, checks, fixed(context)));
  const grade = await gradeTrajectory(trajectory,
    [{ type: "event-task" }, { type: "event-route" }, { type: "no-polling" }, { type: "semantic-review" }], { registry });
  assert.equal(grade.passed, false);
  assert.equal(grade.score, 3 / 4);
  assert.deepEqual(grade.details?.map(result => result.passed), [true, true, true, false]);
});

test("a passing semantic opinion cannot override a mechanical failure", async () => {
  const context = evidence();
  context.calls = [];
  const registry = createGraderRegistry();
  const checks = registerEventGraders(registry, fixed(context));
  registry.register(new SemanticReviewGrader(async () => "No additional concerns.\nVerdict: PASS", checks, fixed(context)));
  const grade = await gradeTrajectory(trajectory,
    [{ type: "event-task" }, { type: "event-route" }, { type: "no-polling" }, { type: "semantic-review" }], { registry });
  assert.equal(grade.passed, false);
});

test("the prose reviewer sees only the deterministic checks the stimulus selected", async () => {
  const context = evidence();
  const checks = registerEventGraders(createGraderRegistry(), fixed(context));
  const stimulus = { name: "raw-api", prompt: "p", graders: [{ type: "event-task" }, { type: "semantic-review" }] };
  await new SemanticReviewGrader(async request => {
    assert.match(request.prompt, /"name":"event-task"/);
    assert.doesNotMatch(request.prompt, /"name":"event-route"|"name":"no-polling"/);
    return "The agent used the raw stream as asked.\nVerdict: PASS";
  }, checks, fixed(context)).grade({ trajectory, stimulus });
});

test("the prose reviewer is told when the harness cancelled the agent at its budget", async () => {
  await new SemanticReviewGrader(async request => {
    assert.match(request.prompt, /How the run ended: agent_timeout/);
    assert.match(request.prompt, /no opportunity to act after the cancellation/);
    return "The agent was cut off while waiting.\nVerdict: FAIL";
  }, [], fixed(evidence())).grade({ trajectory: { ...trajectory, endReason: "agent_timeout" } });
});

test("the prose reviewer sees late trajectory patterns and long outputs without the old truncation", async () => {
  const full: Trajectory = {
    ...trajectory,
    events: Array.from({ length: 450 }, (_, index) => ({
      type: "reasoning", data: { content: index === 449 ? "x".repeat(5_000) + "LATE-SURPRISING-PATTERN" : `step-${index}` },
    })),
  };
  await new SemanticReviewGrader(async request => {
    assert.match(request.prompt, /step-0/);
    assert.match(request.prompt, /LATE-SURPRISING-PATTERN/);
    return "A late pattern may indicate a gap in the automated checks.\nVerdict: FAIL";
  }, [], fixed(evidence())).grade({ trajectory: full });
});

for (const prose of ["", "Everything looks good.", "Verdict: PASS", '{"passed":true}']) {
  test(`a missing prose explanation/verdict is a grading error: ${JSON.stringify(prose)}`, async () => {
    await assert.rejects(new SemanticReviewGrader(async () => prose, [], fixed(evidence())).grade({ trajectory }),
      /must explain its findings/);
  });
}

test("definitions select four independent graders, equal default weights, and unchanged prompts/budgets", async () => {
  for (const scenario of scenarioNames) {
    const spec = await loadEvalSpec(definitionPath("events", scenario));
    assert.equal(spec.name, scenario);
    const executor = spec.defaults.executor;
    assert.ok(typeof executor === "object" && executor.name === "gh-agent");
    assert.deepEqual(parseExecutorConfig(executor.config, suites), { suite: suites.events, scenario });
    assert.equal(spec.defaults.timeout, "10m");
    assert.equal(spec.scoring?.threshold, 1);
    assert.equal(spec.scoring?.weights, undefined);
    for (const [name, variant] of [[scenario, "eval"], [`${scenario}-design-probe`, "design-probe"]] as const) {
      const stimulus = spec.stimuli.find(candidate => candidate.name === name);
      assert.ok(stimulus);
      assert.deepEqual(stimulus.tags, { variant });
      assert.equal(stimulus.constraints?.max_agent_duration, "5m");
      assert.deepEqual(stimulus.graders?.map(grader => grader.type), ["event-task", "event-route", "no-polling", "semantic-review"]);
      assert.ok(stimulus.rubric && stimulus.rubric.length >= 5);
      assert.equal(stimulus.prompt!.includes(prUrlPlaceholder), scenario !== "full-delegation");
    }
    // The command observer cannot see HTTP clients such as curl, so raw API trials only get outcome and semantic grading.
    const raw = spec.stimuli.find(candidate => candidate.name === `${scenario}-raw-api`);
    assert.ok(raw);
    assert.deepEqual(raw.tags, { variant: "raw-api" });
    assert.equal(raw.constraints?.max_agent_duration, "5m");
    assert.deepEqual(raw.graders?.map(grader => grader.type), ["event-task", "semantic-review"]);
    assert.match(raw.prompt!, /POST https:\/\/api\.github\.com\/event-subscriptions/);
    assert.doesNotMatch(raw.prompt!, /gh event/);
  }
  const spec = await loadEvalSpec(definitionPath("events", "notify-review"));
  assert.doesNotMatch(spec.stimuli[0]!.prompt!, /event|poll|gh /);
  assert.match(spec.stimuli.find(candidate => candidate.name === "notify-review-design-probe")!.prompt!,
    /no command syntax is provided/);
  assert.equal(resolvePrompt(`Review ${prUrlPlaceholder}`, { PR_URL: prUrl }), `Review ${prUrl}`);
  assert.throws(() => resolvePrompt(`Review ${prUrlPlaceholder}`, {}), /references \{\{PR_URL\}\}, but this fixture does not provide it/);
  assert.throws(() => parseExecutorConfig({ suite: "nope", scenario: "notify-review" }, suites), /config.suite must be one of: events/);
  assert.throws(() => parseExecutorConfig({ suite: "events", scenario: "nope" }, suites), /config.scenario must be one of/);
  assert.throws(() => parseExecutorConfig({ suite: "events", scenario: "notify-review", extra: 1 }, suites), /unknown field/);
});

test("vally lint accepts every definition with the grader plugin loaded", { timeout: 60_000 }, async () => {
  const vally = fileURLToPath(new URL("../node_modules/.bin/vally", import.meta.url));
  const plugin = fileURLToPath(new URL("./grader-plugin.js", import.meta.url));
  for (const scenario of scenarioNames) {
    const { stdout, stderr } = await exec(vally, ["lint", "--eval-spec", definitionPath("events", scenario), "--grader-plugin", plugin]);
    assert.doesNotMatch(stdout + stderr, /\berror\b/i);
  }
});

test("recorded evidence retains metadata and rejects incomplete or old output-free captures", async t => {
  const root = await mkdtemp(join(tmpdir(), "gh-evidence-"));
  t.after(() => rm(root, { recursive: true }));
  const context = evidence();
  context.snapshot.reviews[0]!.body = "Keep the complete review";
  context.snapshot.pr!.title = "Keep the complete PR";
  const saved = { ...trajectory, artifactDir: root };
  await writeFile(join(root, evidenceFile), JSON.stringify(context));
  assert.deepEqual(await readTrialEvidence(saved), context);
  const previous = { ...context, version: 2 };
  delete (previous as Partial<Evidence>).variant;
  await writeFile(join(root, evidenceFile), JSON.stringify(previous));
  assert.deepEqual(await readTrialEvidence(saved), {
    ...context,
    variant: "unknown",
  });
  for (const invalid of [{ ...context, version: 1 }, { ...context, snapshot: {} },
    { ...context, calls: [{ ...wait(), output: undefined }] }]) {
    await writeFile(join(root, evidenceFile), JSON.stringify(invalid));
    await assert.rejects(evidenceLoader()({ trajectory: saved }), /Malformed gh-event-evidence/);
  }
  await assert.rejects(evidenceLoader()({ trajectory }), /No artifact directory/);
});

test("stream events using GitHub's numeric repository URL prove the fixture review was observed", async () => {
  const context = evidence();
  context.calls = [{
    ...wait(),
    arguments: ["event", "listen", "--resource", prUrl, "--event", "review,state"],
    output: [{ stream: "stdout", at: 15, text: JSON.stringify({
      specversion: "1.0", id: "event-review", type: "com.github.pull_request.review.submitted.v1",
      source: "https://api.github.com/repositories/1439575363", subject: "pull_requests/5082851093",
      data: { api_url: "https://api.github.com/repositories/1439575363/pulls/1/reviews/7" },
    }) + "\n" }],
  }];
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, true);
  (context.calls[0] as GhCall).output[0]!.text = (context.calls[0] as GhCall).output[0]!.text.replace("/reviews/7", "/reviews/8");
  assert.equal((await new EventRouteGrader(fixed(context)).grade({ trajectory })).passed, false,
    "an event for another review must not satisfy the scripted review");
});

function call(id: string, at: number, ...args: string[]): GhCall {
  return { id, kind: "gh", arguments: args, startedAt: at, endedAt: at + 0.5, success: true, exitCode: 0, output: [] };
}

test("at the subscription limit, freeing any slot and observing the review passes and reports what was deleted", async () => {
  const context = evidence();
  context.scenario = "listen-at-limit";
  const limited = { ...call("limited", 0.1, "event", "listen", "--resource", prUrl, "--event", "review"), success: false, exitCode: 1 };
  context.calls = [
    limited,
    call("list", 0.2, "event", "subscription", "ls", "--json", "id,resourceUrl"),
    call("delete", 0.3, "event", "subscription", "delete", "sub_a1"),
    ...context.calls,
  ];
  context.snapshot.subscriptions = { stale: ["sub_s1"], active: ["sub_a1"], remaining: ["sub_s1"] };
  const task = () => new EventTaskGrader(fixed(context)).grade({ trajectory });
  const passed = await task();
  assert.equal(passed.passed, true);
  assert.deepEqual(passed.metadata, { staleDeleted: 0, activeDeleted: 1, stale: 1, active: 1 });
  const routed = await new EventRouteGrader(fixed(context)).grade({ trajectory });
  assert.equal(routed.passed, true);
  assert.deepEqual(routed.metadata?.subscriptionCommands, { list: 1, delete: 1 });

  delete context.snapshot.subscriptions;
  assert.match((await task()).evidence, /seeded subscriptions were not recorded/);
});
