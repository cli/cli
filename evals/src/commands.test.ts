import assert from "node:assert/strict";
import { test } from "node:test";
import { actorCatchUpLimitMs, actorReactionDelayMs, attachedSince, attachments, command, dedicatedWait, persistentWait, readyForActor, statusRead } from "./suites/events/commands.js";
import type { GhCall } from "./core/calls.js";

function call(args: string[]): GhCall {
  return { id: "gh:control", kind: "gh", arguments: args, startedAt: 0, output: [], success: true };
}

test("wait recognition uses executed argv, including global flags, not snippets or help text", () => {
  for (const args of [
    ["event", "listen", "--resource", "https://github.com/sandbox/run/pull/1", "--event", "review", "--count", "1"],
    ["-R", "sandbox/run", "event", "listen", "--subscription", "sub_x"],
    ["event", "listen", "--resource=https://github.com/sandbox/run/pull/1", "--event", "review,state"],
  ]) assert.ok(dedicatedWait(call(args)), args.join(" "));
  for (const args of [
    ["api", "/event-subscriptions/sub_x/events"], ["event", "listen", "--help"], ["event"],
    ["pr", "checks", "--watch"], ["run", "watch"], ["pr", "view", "--template", "gh event listen"],
  ]) assert.equal(dedicatedWait(call(args)), undefined, args.join(" "));
  assert.equal(dedicatedWait({ id: "tool:x", kind: "tool", arguments: { command: "gh event listen --subscription sub_x" }, startedAt: 0 }), undefined);
});

test("a listener counts as attached only while its stderr reports an attached stream", () => {
  const listen = call(["event", "listen", "--subscription", "sub_x"]);
  assert.equal(attachedSince(listen), undefined);
  listen.output.push({ stream: "stdout", at: 1, text: "Listening for events.\n" });
  listen.output.push({ stream: "stderr", at: 2, text: "Could not attach the event stream, retrying: HTTP 503\nListening for" });
  assert.equal(attachedSince(listen), undefined);
  listen.output.push({ stream: "stderr", at: 3, text: " events.\n" });
  assert.equal(attachedSince(listen), 3);
  listen.output.push({ stream: "stderr", at: 4, text: "Event stream closed, reconnecting.\n" });
  assert.equal(attachedSince(listen), undefined, "a reconnecting listener has no stream attached");
  listen.output.push({ stream: "stderr", at: 5, text: "Listening for events.\n" });
  assert.equal(attachedSince(listen), 5);
  assert.deepEqual(attachments(listen).at, [3, 5]);
});

test("the second actor waits for the agent's catch-up read after a listener attaches", () => {
  const listen = call(["event", "listen", "--resource", "https://github.com/sandbox/run/pull/1", "--follow"]);
  listen.output.push({ stream: "stderr", at: 1_000, text: "Listening for events.\n" });
  const settled = 1_000 + actorReactionDelayMs;
  assert.equal(readyForActor(listen, [listen], settled), false, "no catch-up read yet");
  const before = { ...call(["pr", "view", "1", "--json", "reviews"]), id: "gh:before", startedAt: 500, endedAt: 900 };
  assert.equal(readyForActor(listen, [listen, before], settled), false, "a read before attachment is not a catch-up");
  const running = { ...call(["pr", "view", "1", "--json", "reviews"]), id: "gh:read", startedAt: 1_500 };
  assert.equal(readyForActor(listen, [listen, running], settled), false, "the catch-up read is still running");
  const identify = { ...call(["pr", "view", "1", "--json", "number"]), id: "gh:id", startedAt: 1_500, endedAt: 1_800 };
  assert.equal(readyForActor(listen, [listen, identify], settled), false, "identification is not a status read");
  const read = { ...running, endedAt: 2_000 };
  assert.equal(readyForActor(listen, [listen, read], 1_000 + actorReactionDelayMs - 1), false, "reaction delay");
  assert.equal(readyForActor(listen, [listen, read], settled), true);
  assert.equal(readyForActor(listen, [listen], 1_000 + actorCatchUpLimitMs), true, "agents that never read still get the event");
});

test("a listener is persistent with --follow or a count above one", () => {
  for (const args of [["--follow"], ["--count", "2"], ["--count=3"]]) {
    assert.ok(persistentWait(call(["event", "listen", "--subscription", "sub_x", ...args])), args.join(" "));
  }
  for (const args of [[], ["--count", "1"], ["--follow=false"]]) {
    assert.equal(persistentWait(call(["event", "listen", "--subscription", "sub_x", ...args])), false, args.join(" "));
  }
});

test("status projections distinguish identification, different fields, reads, and mutations", () => {
  assert.equal(statusRead(call(["pr", "view", "--json", "url,number"])), undefined);
  for (const field of ["latestReviews", "closed", "closedAt"]) assert.ok(statusRead(call(["pr", "view", "--json", field])), field);
  assert.equal(statusRead(call(["api", "--method", "POST", "/repos/o/r/pulls/1"])), undefined);
  assert.ok(statusRead(call(["api", "-XGET", "/repos/o/r/pulls/1"])));
  assert.equal(statusRead(call(["pr", "view", "--json", "reviews,state"])),
    statusRead(call(["pr", "view", "--json=state,reviews"])));
  assert.deepEqual(command(["-Ro/r", "event", "listen", "--count", "1", "--timeout=5m"]).words, ["event", "listen"]);
});

test("event types are collected from repeated --event flags and the earlier --events spelling", () => {
  const args = ["event", "listen", "--event", "a,b", "--event=c", "--events", "d"];
  assert.equal(command(args).options.get("--event"), "a,b,c,d");
});

test("a GraphQL status read is recognized wherever its query field sits among repeated fields", () => {
  const query = "query=query($o:String!){repository(owner:$o,name:\"r\"){pullRequest(number:1){reviewDecision}}}";
  const expected = statusRead(call(["api", "graphql", "-f", query, "-f", "o=sandbox"]));
  assert.ok(expected);
  assert.equal(statusRead(call(["api", "graphql", "-f", query, "-f", "o=sandbox", "-F", "n=1"])), expected);
  assert.equal(statusRead(call(["api", "graphql", "-F", "o=sandbox", `-f${query}`])), expected);
  assert.equal(statusRead(call(["api", "graphql", "--raw-field", "o=sandbox", `--field=${query}`])), expected);
});

test("GraphQL status reads of different pull requests with the same query are told apart", () => {
  const query = "query=query($o:String!,$n:Int!){repository(owner:$o,name:\"r\"){pullRequest(number:$n){reviewDecision}}}";
  const first = statusRead(call(["api", "graphql", "-f", query, "-F", "o=sandbox", "-F", "n=1"]));
  assert.ok(first);
  assert.equal(statusRead(call(["api", "graphql", "-F", "n=1", "-f", query, "-F", "o=sandbox"])), first);
  assert.notEqual(statusRead(call(["api", "graphql", "-f", query, "-F", "o=sandbox", "-F", "n=2"])), first);
  assert.notEqual(statusRead(call(["api", "graphql", "-f", query, "-F", "o=other", "-F", "n=1"])), first);
});
