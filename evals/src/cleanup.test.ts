import assert from "node:assert/strict";
import { test } from "node:test";
import { Actor } from "./suites/events/actor.js";
import { ActorDriver } from "./core/actor.js";
import { GhAgentExecutor } from "./core/executor.js";
import { suites } from "./suites/index.js";
import { EventsGitHub } from "./suites/events/github.js";
import { cleanupTrial } from "./core/trial-cleanup.js";
import type { Fixture } from "./suites/events/domain.js";

function actions() {
  return {
    actor: async () => {},
    agent: async () => {},
    observer: async () => {},
    resources: async () => {},
    local: async () => {},
  };
}

test("an actor request failure is reported once when resource cleanup succeeds", async () => {
  const reset = Object.assign(new Error("read ECONNRESET"), { code: "ECONNRESET" });
  const github = new EventsGitHub("test-only", async () => { throw reset; }, []);
  const fixture: Fixture = {
    scenario: "notify-review", repo: "sandbox/run", workDir: "/unused", baseSha: "base", initialHead: "head",
    pr: {
      number: 1, html_url: "https://github.com/sandbox/run/pull/1", state: "open", merged: false,
      mergeable: true, merge_commit_sha: null, head: { sha: "head", ref: "topic" }, base: { ref: "main" },
    },
  };
  const driver = new ActorDriver(new Actor(github, github, fixture, 100, 0), async () => {}, () => 10);
  await assert.rejects(driver.tick(), /ECONNRESET/);
  const primary = driver.error;
  assert.ok(primary);
  let resourcesCleaned = false;
  let localCleaned = false;
  async function trial() {
    try { throw primary; }
    finally {
      await cleanupTrial({
        ...actions(),
        actor: () => driver.close(),
        resources: async () => { resourcesCleaned = true; },
        local: async () => { localCleaned = true; },
      }, primary, "/trial/resources.json");
    }
  }
  await assert.rejects(trial(), error => {
    assert.equal(error, primary, "successful cleanup must preserve the original actor error, not manufacture a shutdown failure");
    assert.doesNotMatch(error.message, /Second actor shutdown|Retry with|evals\/cleanup/);
    return true;
  });
  assert.equal(resourcesCleaned, true);
  assert.equal(localCleaned, true);
});

for (const [phase, label] of [
  ["agent", "Copilot executor shutdown"],
  ["observer", "Command observer cleanup"],
  ["local", "Local cleanup"],
] as const) {
  test(`${phase} teardown errors do not advise retrying successful resource cleanup`, async () => {
    const cleanup = actions();
    cleanup[phase] = async () => { throw new Error("teardown failed"); };
    await assert.rejects(cleanupTrial(cleanup, undefined, "/trial/resources.json"), error => {
      assert.ok(error instanceof Error);
      assert.ok(error.message.includes(label));
      assert.doesNotMatch(error.message, /Retry with|evals\/cleanup/);
      return true;
    });
  });
}

test("a real resource cleanup failure retains the execution error and actionable retry path", async () => {
  const primary = new Error("execution failed");
  let removedLocal = false;
  await assert.rejects(cleanupTrial({
    ...actions(),
    resources: async () => { throw new Error("GitHub DELETE: HTTP 403"); },
    local: async () => { removedLocal = true; },
  }, primary, "/trial/resources.json"), error => {
    assert.ok(error instanceof Error);
    assert.match(error.message, /execution failed/);
    assert.match(error.message, /Resource cleanup:.*HTTP 403/);
    assert.match(error.message, /Retry with: evals\/cleanup \/trial\/resources\.json/);
    assert.equal(error.cause, primary);
    return true;
  });
  assert.equal(removedLocal, true, "failed resource deletion must not prevent local cleanup");
});

test("a new actor failure first encountered at shutdown is not suppressed", async () => {
  await assert.rejects(cleanupTrial({
    ...actions(), actor: async () => { throw new Error("late actor failure"); },
  }, undefined, "/trial/resources.json"), error => {
    assert.ok(error instanceof Error);
    assert.match(error.message, /Second actor execution:.*late actor failure/);
    assert.doesNotMatch(error.message, /Retry with|evals\/cleanup/);
    return true;
  });
});

test("deduplicating an actor error never suppresses a real resource cleanup failure with the same cause", async () => {
  const primary = new Error("connection lost");
  await assert.rejects(cleanupTrial({
    ...actions(),
    actor: async () => { throw primary; },
    resources: async () => { throw primary; },
  }, primary, "/trial/resources.json"), error => {
    assert.ok(error instanceof Error);
    assert.match(error.message, /Resource cleanup:.*connection lost/);
    assert.match(error.message, /Retry with: evals\/cleanup/);
    assert.doesNotMatch(error.message, /Second actor execution/);
    return true;
  });
});

test("shutdown waits for the running trial to clean up and refuses trials not yet started", async () => {
  const executor = new GhAgentExecutor(suites);
  let release!: () => void;
  const cleanedUp = new Promise<void>(resolve => { release = resolve; });
  let started = 0;
  Object.assign(executor, { trial: async () => { started++; await cleanedUp; return {}; } });
  const options = {} as Parameters<GhAgentExecutor["execute"]>[1];
  const stimulus = {} as Parameters<GhAgentExecutor["execute"]>[0];
  const running = executor.execute(stimulus, options);
  const queued = executor.execute(stimulus, options);
  await new Promise(resolve => setImmediate(resolve));
  let stopped = false;
  const shutdown = executor.shutdown().then(() => { stopped = true; });
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(stopped, false, "shutdown must not resolve while a trial is still cleaning up");
  release();
  await shutdown;
  await running;
  await assert.rejects(queued, /interrupted before this trial started/);
  assert.equal(started, 1);
});
