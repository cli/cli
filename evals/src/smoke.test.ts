import assert from "node:assert/strict";
import { test } from "node:test";
import {
  ReviewEventCollector, runSmokeProtocol, runSubscriptionCommands, StreamReadySignal, type GhRunner, type SmokeProtocol,
} from "./suites/events/smoke-protocol.js";

test("the scripted smoke performs two fixes and observes each review before continuing", async () => {
  const calls: string[] = [];
  let head = "initial";
  let nextReview = 100;
  const protocol: SmokeProtocol = {
    async currentHead() { calls.push("head"); return head; },
    async submitReview(reviewedHead, event) {
      assert.equal(reviewedHead, head);
      calls.push(`submit:${event}:${reviewedHead}`);
      return nextReview++;
    },
    async waitForReview(id) { calls.push(`wait:${id}`); },
    async pushGreeting(greeting) {
      calls.push(`push:${greeting.trim()}`);
      head = `head-${calls.filter(call => call.startsWith("push:")).length}`;
      return head;
    },
    async verify(finalHead, reviews) {
      calls.push(`verify:${finalHead}:${reviews.join(",")}`);
      assert.equal(finalHead, head);
    },
  };
  const result = await runSmokeProtocol(protocol);
  assert.deepEqual(result, {
    initialHead: "initial", firstFix: "head-1", finalHead: "head-2", reviews: [100, 101, 102],
  });
  assert.deepEqual(calls, [
    "head",
    "submit:REQUEST_CHANGES:initial", "wait:100", "push:Hello, world!",
    "submit:REQUEST_CHANGES:head-1", "wait:101", "push:Hello, GitHub!",
    "submit:APPROVE:head-2", "wait:102", "verify:head-2:100,101,102",
  ]);
});

test("the event collector handles split JSONL, unrelated events, and events received before waiting", async () => {
  const collector = new ReviewEventCollector();
  collector.write('{"type":"state","data":{"api_url":"https://api.github.com/repositories/1/pulls/1"}}\n');
  collector.write('{"type":"review","data":{"api_url":"https://api.github.com/repositories/1/pulls/1/revi');
  collector.write('ews/42"}}\n{"type":"review","data":{"api_url":"https://api.github.com/repos/o/r/pulls/1/reviews/43"}}\n');
  await collector.wait(42, 10);
  await collector.wait(43, 10);
  await assert.rejects(collector.wait(44, 5), /Timed out waiting for review event 44/);
});

test("malformed stream output fails pending and later waits instead of throwing from the stream callback", async () => {
  const collector = new ReviewEventCollector();
  const pending = collector.wait(42, 1_000);
  assert.doesNotThrow(() => collector.write("not json\n"));
  await assert.rejects(pending, /not JSON: not json/);
  await assert.rejects(collector.wait(43, 1_000), /not JSON/);

  const review = (id: number) => `{"type":"review","data":{"api_url":"https://api.github.com/repos/o/r/pulls/1/reviews/${id}"}}\n`;
  const received = new ReviewEventCollector();
  received.write(`${review(42)}not json\n`);
  await assert.rejects(received.wait(42, 1_000), /not JSON/, "a review already received does not hide later bad output");
  const awaited = new ReviewEventCollector();
  const final = awaited.wait(43, 1_000);
  awaited.write(`${review(43)}not json\n`);
  await assert.rejects(final, /not JSON/, "bad output in the chunk that resolves the wait fails it");
  assert.throws(() => awaited.check(), /not JSON/);
});

test("the ready signal waits for the command's explicit attachment diagnostic", async () => {
  const ready = new StreamReadySignal();
  ready.write("unrelated stderr\nListening for ");
  const waiting = ready.wait(100);
  ready.write("events.\n");
  await waiting;
  await ready.wait(1);
});

test("the subscription smoke creates, views, lists, and deletes a subscription, then confirms it is gone", async () => {
  const prUrl = "https://github.com/sandbox/run/pull/1";
  const type = "com.github.pull_request.merge_conflict.v1";
  const held = new Set<string>();
  const ran: string[] = [];
  const gh: GhRunner = async args => {
    ran.push(args.slice(0, 3).join(" "));
    const [, , sub, id] = args;
    const ok = (stdout: string) => ({ code: 0, stdout, stderr: "" });
    if (sub === "create") { held.add("sub_new"); return ok("sub_new\n"); }
    if (sub === "list") return ok(JSON.stringify([...held].map(held => ({ id: held }))));
    if (sub === "delete") { held.delete(id!); return ok(""); }
    if (sub === "view" && held.has(id!)) return ok(JSON.stringify({ id, resourceUrl: prUrl, eventTypes: [type] }));
    return { code: 1, stdout: "", stderr: `subscription ${id} was not found; it may have expired` };
  };
  assert.equal(await runSubscriptionCommands(gh, prUrl), "sub_new");
  assert.deepEqual(ran, ["event subscription create", "event subscription view", "event subscription list",
    "event subscription delete", "event subscription view"]);

  const undeleted: GhRunner = async args => args[2] === "delete" ? { code: 0, stdout: "", stderr: "" } : gh(args);
  held.clear();
  await assert.rejects(runSubscriptionCommands(undeleted, prUrl), /still found sub_new after deletion/);
});
