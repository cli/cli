import assert from "node:assert/strict";
import { test } from "node:test";
import { EventsGitHub } from "./suites/events/github.js";

const reset = () => Object.assign(new Error("read ECONNRESET"), { code: "ECONNRESET" });

test("a connection reset on a GET is retried once and can recover", async () => {
  let calls = 0;
  const github = new EventsGitHub("test-only", async () => {
    if (++calls === 1) throw reset();
    return Response.json({ number: 1 });
  }, [0, 0, 0]);
  assert.deepEqual(await github.api("GET", "/repos/sandbox/run/pulls/1"), { number: 1 });
  assert.equal(calls, 2);
});

test("a transient 5xx on any GET is retried with backoff and can recover", async () => {
  const statuses = [503, 500, 502, 200];
  let calls = 0;
  const github = new EventsGitHub("test-only", async () => {
    const status = statuses[calls++]!;
    return status === 200 ? Response.json({ number: 1 }) : new Response("{}", { status });
  }, [0, 0, 0]);
  assert.deepEqual(await github.api("GET", "/repos/sandbox/run/pulls/1"), { number: 1 });
  assert.equal(calls, 4);
  const down = new EventsGitHub("test-only", async () => new Response("{}", { status: 503 }), [0, 0]);
  await assert.rejects(down.api("GET", "/repos/sandbox/run/pulls/1"), /HTTP 503/);
});

test("idempotent writes retry transient statuses, while expected 4xx results stay final", async () => {
  const calls: string[] = [];
  const github = new EventsGitHub("test-only", async (_input, init) => {
    calls.push(init!.method!);
    if (init!.method === "DELETE") return new Response("{}", { status: 404 });
    return calls.length === 1 ? new Response("{}", { status: 429 }) : Response.json({ merged: true });
  }, [0, 0, 0]);
  assert.deepEqual(await github.api("PUT", "/repos/sandbox/run/pulls/1/merge", {}), { merged: true });
  await assert.rejects(github.api("DELETE", "/event-subscriptions/sub_gone"), /HTTP 404/);
  assert.deepEqual(calls, ["PUT", "PUT", "DELETE"]);
});

test("persistent GET resets stop after one retry and name the method and path", async () => {
  let calls = 0;
  const failure = reset();
  const github = new EventsGitHub("test-only", async () => { calls++; throw failure; }, [0, 0, 0]);
  await assert.rejects(github.api("GET", "/repos/sandbox/run/pulls/1"), error => {
    assert.ok(error instanceof Error);
    assert.match(error.message, /GitHub GET \/repos\/sandbox\/run\/pulls\/1:.*ECONNRESET/);
    assert.equal(error.cause, failure);
    return true;
  });
  assert.equal(calls, 2);
});

test("a reset after a review POST is contextualized but never retried", async () => {
  let calls = 0;
  const failure = reset();
  const github = new EventsGitHub("test-secret-token", async () => { calls++; throw failure; }, [0, 0]);
  await assert.rejects(github.api("POST", "/repos/sandbox/run/pulls/1/reviews", { event: "COMMENT" }), error => {
    assert.ok(error instanceof Error);
    assert.match(error.message, /GitHub POST \/repos\/sandbox\/run\/pulls\/1\/reviews:.*ECONNRESET/);
    assert.doesNotMatch(error.message, /test-secret-token/);
    assert.equal(error.cause, failure);
    return true;
  });
  assert.equal(calls, 1, "an ambiguous POST result must not produce duplicate reviews");
});

test("fetch-style nested connection resets can recover without retrying other transport errors", async () => {
  let calls = 0;
  const github = new EventsGitHub("test-only", async () => {
    if (++calls === 1) throw new TypeError("fetch failed", { cause: reset() });
    return Response.json({ ok: true });
  }, [0]);
  assert.deepEqual(await github.api("GET", "/repos/sandbox/run"), { ok: true });
  const dns = Object.assign(new Error("DNS lookup failed"), { code: "ENOTFOUND" });
  const invalid = new EventsGitHub("test-only", async () => { throw dns; }, [0]);
  await assert.rejects(invalid.api("GET", "/repos/sandbox/run"), /GitHub GET.*ENOTFOUND/);
});

test("listing subscriptions follows Link pagination and rejects links that leave the listing", async () => {
  const sub = (n: number) => ({ id: `sub_${n}`, resource_url: `https://github.com/sandbox/run/pull/${n}`, event_types: [] });
  const paths: string[] = [];
  const github = new EventsGitHub("test-only", async input => {
    const url = new URL(String(input));
    paths.push(url.pathname + url.search);
    return url.searchParams.get("page") === "2"
      ? Response.json({ subscriptions: [sub(2)] })
      : Response.json({ subscriptions: [sub(1)] }, { headers: {
        Link: '<https://api.github.com/event-subscriptions?per_page=100&page=2>; rel="next", '
          + '<https://api.github.com/event-subscriptions?per_page=100&page=2>; rel="last"',
      } });
  });
  assert.deepEqual((await github.subscriptions()).map(subscription => subscription.id), ["sub_1", "sub_2"]);
  assert.deepEqual(paths, ["/event-subscriptions?per_page=100", "/event-subscriptions?per_page=100&page=2"]);
  const escaping = new EventsGitHub("test-only", async () => Response.json({ subscriptions: [] }, {
    headers: { Link: '<https://api.github.com/repos/sandbox/run/pulls?page=2>; rel="next"' },
  }));
  await assert.rejects(escaping.subscriptions(), /Unexpected event subscription pagination link/);
});
