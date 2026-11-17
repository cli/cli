import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { replay } from "./replay.js";

const exec = promisify(execFile);
const fixture = fileURLToPath(new URL("../testdata/replays/persistent-review-loop.json", import.meta.url));

test("offline replay passes the sanitized persistent-stream evidence", async () => {
  const result = await replay(fixture);
  assert.equal(result.passed, true);
  assert.deepEqual(result.graders.map(grader => [grader.name, grader.passed]), [
    ["event-task", true], ["event-route", true], ["no-polling", true],
  ]);
});

test("offline replay catches a missed review and actor-delivery regressions", async t => {
  const root = await mkdtemp(join(tmpdir(), "gh-replay-"));
  t.after(() => rm(root, { recursive: true }));
  const original = JSON.parse(await readFile(fixture, "utf8"));
  const missed = structuredClone(original);
  missed.calls.find((call: { id: string }) => call.id === "gh:snapshot").output[0].text =
    JSON.stringify({ reviewDecision: "", headRefOid: "initial", reviews: [] }) + "\n";
  const missedPath = join(root, "missed.json");
  await writeFile(missedPath, JSON.stringify(missed));
  const missedResult = await replay(missedPath);
  assert.equal(missedResult.graders.find(grader => grader.name === "event-route")?.passed, false);

  const wrongDelivery = structuredClone(original);
  wrongDelivery.snapshot.actions.find((action: { kind: string }) => action.kind === "approval").trigger = "wait";
  const deliveryPath = join(root, "delivery.json");
  await writeFile(deliveryPath, JSON.stringify(wrongDelivery));
  const deliveryResult = await replay(deliveryPath);
  assert.equal(deliveryResult.graders.find(grader => grader.name === "event-route")?.passed, false);
});

test("offline replay skips the route and polling checks for raw-API trials", async t => {
  const root = await mkdtemp(join(tmpdir(), "gh-replay-"));
  t.after(() => rm(root, { recursive: true }));
  const rawAPI = { ...JSON.parse(await readFile(fixture, "utf8")), version: 3, variant: "raw-api" };
  // A delivery the route check rejects, which a raw-API trial must not be failed for.
  rawAPI.snapshot.actions.find((action: { kind: string }) => action.kind === "approval").trigger = "wait";
  const path = join(root, "raw-api.json");
  await writeFile(path, JSON.stringify(rawAPI));

  const result = await replay(path);

  assert.equal(result.passed, true);
  assert.deepEqual(result.graders.map(grader => [grader.name, grader.passed]), [["event-task", true]]);
});

test("the replay CLI accepts an artifact directory and uses no credential environment", async t => {
  const root = await mkdtemp(join(tmpdir(), "gh-replay-cli-"));
  t.after(() => rm(root, { recursive: true }));
  await writeFile(join(root, "gh-event-evidence.json"), await readFile(fixture));
  const script = fileURLToPath(new URL("./replay.js", import.meta.url));
  const { stdout } = await exec(process.execPath, [script, root], {
    env: { PATH: process.env.PATH, HOME: root },
  });
  assert.match(stdout, /^PASS review-loop/m);
  assert.match(stdout, /PASS event-route/);
  assert.match(stdout, /PASS no-polling/);
});
