import assert from "node:assert/strict";
import { test, type TestContext } from "node:test";
import { access } from "node:fs/promises";
import type { CopilotClientOptions, SessionConfig } from "@github/copilot-sdk";
import { copilotReviewer } from "./graders/copilot-review.js";

function token(t: TestContext): void {
  const previous = process.env.COPILOT_GITHUB_TOKEN;
  process.env.COPILOT_GITHUB_TOKEN = "test-model-token";
  t.after(() => {
    if (previous === undefined) delete process.env.COPILOT_GITHUB_TOKEN;
    else process.env.COPILOT_GITHUB_TOKEN = previous;
  });
}

test("the prose reviewer uses the selected model in an isolated tool-free session and removes its home", async t => {
  token(t);
  let home: string | undefined;
  let stopped = 0;
  let config: SessionConfig | undefined;
  const review = copilotReviewer((options: CopilotClientOptions) => {
    home = options.env?.HOME;
    assert.equal(options.useLoggedInUser, false);
    assert.equal(options.gitHubToken, "test-model-token");
    assert.equal(options.env?.GH_TOKEN, undefined);
    assert.equal(options.env?.GH_EVAL_ACTOR_TOKEN, undefined);
    return {
      async createSession(value) {
        assert.ok(options.env?.TMPDIR);
        await access(options.env.TMPDIR);
        config = value;
        return {
          async sendAndWait(prompt, timeout) {
            assert.equal(prompt, "Review this trajectory.");
            assert.equal(timeout, 5_000);
            return { data: { content: "The agent ignored the review.\nVerdict: FAIL" } };
          },
        };
      },
      async stop() { stopped++; return []; },
    };
  });
  assert.equal(await review({ model: "selected-model", prompt: "Review this trajectory.", timeoutMs: 5_000 }),
    "The agent ignored the review.\nVerdict: FAIL");
  assert.ok(home);
  assert.equal(config?.model, "selected-model");
  assert.equal(config?.enableConfigDiscovery, false);
  assert.equal(config?.requestExtensions, false);
  assert.deepEqual(config?.availableTools, []);
  assert.equal(stopped, 1);
  await assert.rejects(access(home), { code: "ENOENT" });
});

test("cancelling a prose review stops the client and cleans up without waiting for a model response", async t => {
  token(t);
  const controller = new AbortController();
  let entered!: () => void;
  const started = new Promise<void>(resolve => { entered = resolve; });
  let stopped = false;
  let home: string | undefined;
  const review = copilotReviewer(options => {
    home = options.env?.HOME;
    return {
      async createSession() {
        return { sendAndWait() { entered(); return new Promise(() => {}); } };
      },
      async stop() { stopped = true; return []; },
    };
  });
  const pending = review({ model: "control", prompt: "Review", signal: controller.signal, timeoutMs: 5_000 });
  await started;
  controller.abort(new Error("cancelled-review"));
  await assert.rejects(pending, /cancelled-review/);
  assert.equal(stopped, true);
  assert.ok(home);
  await assert.rejects(access(home), { code: "ENOENT" });
});

test("review startup failure still removes the isolated home", async t => {
  token(t);
  let home: string | undefined;
  const review = copilotReviewer(options => {
    home = options.env?.HOME;
    throw new Error("failed-review-startup");
  });
  await assert.rejects(review({ model: "control", prompt: "Review", timeoutMs: 5_000 }), /failed-review-startup/);
  assert.ok(home);
  await assert.rejects(access(home), { code: "ENOENT" });
});

test("review failure and cleanup failure are both reported", async t => {
  token(t);
  const review = copilotReviewer(() => ({
    async createSession() { throw new Error("model-failed"); },
    async stop() { return [new Error("stop-failed")]; },
  }));
  await assert.rejects(review({ model: "control", prompt: "Review", timeoutMs: 5_000 }), error => {
    assert.ok(error instanceof AggregateError);
    assert.deepEqual(error.errors.map(item => item.message), ["model-failed", "stop-failed"]);
    return true;
  });
});
