import { execFile, spawn, type ChildProcess } from "node:child_process";
import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { homedir, tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import { prepareHome } from "./core/home.js";
import { parseJournal, type Journal } from "./core/sandbox.js";
import { cleanup, provision } from "./suites/events/fixtures.js";
import { EventsGitHub } from "./suites/events/github.js";
import {
  ReviewEventCollector, runSmokeProtocol, runSubscriptionCommands, StreamReadySignal, type GhRunner,
} from "./suites/events/smoke-protocol.js";
import { finalLoopGreeting } from "./suites/events/scenarios/review-loop.js";
import { git, save } from "./core/io.js";

interface Options {
  gh: string;
  output: string;
}

const usage = `Usage: evals/smoke --live --gh PATH [--output DIR]

Run the real review-loop protocol without a model, then create, view, list, and
delete a subscription with gh event subscription. This creates a private sandbox
repository and requires explicit GH_EVAL_HOST, GH_EVAL_ORG, GH_EVAL_TOKEN, and
GH_EVAL_ACTOR_TOKEN. GH_EVAL_FIXTURE_TOKEN is optional.`;

function required(name: string): string {
  const value = process.env[name];
  if (!value?.trim()) throw new Error(`${name} must be supplied explicitly`);
  return value;
}

function parse(argv: string[]): Options | undefined {
  let live = false;
  let gh = "";
  let output = "";
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index]!;
    if (arg === "--help" || arg === "-h") { console.log(usage); return; }
    if (arg === "--live") live = true;
    else if (arg === "--gh") gh = argv[++index] ?? "";
    else if (arg === "--output") output = argv[++index] ?? "";
    else throw new Error(`Unknown option: ${arg}\n${usage}`);
  }
  if (!live) throw new Error(`Live operations require --live.\n${usage}`);
  if (!gh) throw new Error(`--gh requires an already-built binary.\n${usage}`);
  return {
    gh,
    output: output || join(process.env.XDG_STATE_HOME || join(homedir(), ".local/state"),
      "gh-event-evals/smoke", new Date().toISOString().replaceAll(":", "-")),
  };
}

function waitForExit(child: ChildProcess): Promise<{ code: number | null; signal: NodeJS.Signals | null }> {
  return new Promise((resolveExit, reject) => {
    child.once("error", reject);
    child.once("close", (code, signal) => resolveExit({ code, signal }));
  });
}

export async function main(argv = process.argv.slice(2)): Promise<void> {
  const options = parse(argv);
  if (!options) return;
  const host = required("GH_EVAL_HOST");
  if (host !== "github.com") throw new Error("The event preview smoke supports github.com only");
  const org = required("GH_EVAL_ORG");
  const token = required("GH_EVAL_TOKEN");
  const actorToken = required("GH_EVAL_ACTOR_TOKEN");
  if (token === actorToken) throw new Error("GH_EVAL_TOKEN and GH_EVAL_ACTOR_TOKEN must represent different identities");
  const gh = await realpath(options.gh);
  await mkdir(options.output, { recursive: true, mode: 0o700 });
  const journalPath = join(options.output, "resources.json");
  const home = await mkdtemp(join(tmpdir(), "gh-smoke-home-"));
  const workDir = await mkdtemp(join(tmpdir(), "gh-smoke-work-"));
  const request: typeof fetch = fetch;
  let journal: Journal | undefined;
  let stream: ChildProcess | undefined;
  let streamExit: Promise<{ code: number | null; signal: NodeJS.Signals | null }> | undefined;
  let primary: unknown;
  let completed: Awaited<ReturnType<typeof runSmokeProtocol>> | undefined;
  let interrupted: Error | undefined;
  const interrupt = (signal: string) => {
    interrupted ??= new Error(`Smoke interrupted by ${signal}`);
    if (stream && stream.exitCode === null && stream.signalCode === null) stream.kill("SIGTERM");
  };
  const onInterrupt = () => interrupt("SIGINT");
  const onTerminate = () => interrupt("SIGTERM");
  process.once("SIGINT", onInterrupt);
  process.once("SIGTERM", onTerminate);
  const checkInterrupted = () => { if (interrupted) throw interrupted; };
  const fixture = new EventsGitHub(process.env.GH_EVAL_FIXTURE_TOKEN?.trim() || token, request);
  const subscriber = new EventsGitHub(token, request);
  const reviewer = new EventsGitHub(actorToken, request);
  try {
    const env = await prepareHome(home, {
      gh, token, modelToken: "unused-by-smoke", observe: false,
    });
    console.error("smoke: creating sandbox fixture");
    const created = await provision(fixture, org, "review-loop", workDir, env, journalPath, subscriber);
    journal = created.journal;
    const pr = created.fixture.pr;
    if (!pr) throw new Error("Smoke fixture did not create its PR");
    const collector = new ReviewEventCollector();
    const ready = new StreamReadySignal();
    let stderr = "";
    stream = spawn(join(home, "bin/gh"), [
      "event", "listen", "--resource", pr.html_url, "--follow", "--timeout", "2m",
      "--event", "com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1",
    ], { cwd: workDir, env, stdio: ["ignore", "pipe", "pipe"] });
    stream.stdout!.setEncoding("utf8");
    stream.stderr!.setEncoding("utf8");
    stream.stdout!.on("data", chunk => collector.write(String(chunk)));
    stream.stderr!.on("data", chunk => { stderr += String(chunk); ready.write(String(chunk)); });
    streamExit = waitForExit(stream);
    await Promise.race([
      ready.wait(20_000),
      streamExit.then(result => Promise.reject(new Error(
        `Event stream exited before attachment: code=${result.code} signal=${result.signal} ${stderr}`,
      ))),
    ]);
    checkInterrupted();
    if (stream.exitCode !== null) throw new Error(`Event stream exited before the smoke began: ${stderr}`);
    await subscriber.pull(created.fixture.repo, pr.number);
    let checkedOut = false;
    const reviewHeads = new Map<number, string>();
    completed = await runSmokeProtocol({
      currentHead: async () => {
        checkInterrupted();
        const head = (await subscriber.pull(created.fixture.repo, pr.number)).head.sha;
        checkInterrupted();
        return head;
      },
      submitReview: async (head, event, body) => {
        checkInterrupted();
        const review = await reviewer.api<{ id: number }>(
          "POST", `/repos/${created.fixture.repo}/pulls/${pr.number}/reviews`, { commit_id: head, event, body },
        );
        checkInterrupted();
        reviewHeads.set(review.id, head);
        console.error(`smoke: submitted ${event.toLowerCase()} review ${review.id}`);
        return review.id;
      },
      waitForReview: async id => {
        checkInterrupted();
        await Promise.race([
          collector.wait(id, 20_000),
          streamExit!.then(result => Promise.reject(new Error(
            `Event stream exited before review ${id}: code=${result.code} signal=${result.signal} ${stderr}`,
          ))),
        ]);
        checkInterrupted();
        console.error(`smoke: observed review event ${id}`);
      },
      pushGreeting: async (greeting, message) => {
        checkInterrupted();
        if (!checkedOut) {
          await git(workDir, env, "switch", "improve-greeting");
          checkedOut = true;
        }
        await writeFile(join(workDir, "greeting.txt"), greeting);
        await git(workDir, env, "add", "greeting.txt");
        await git(workDir, env, "commit", "-m", message);
        const head = await git(workDir, env, "rev-parse", "HEAD");
        await git(workDir, env, "push", "origin", "improve-greeting");
        checkInterrupted();
        console.error(`smoke: pushed ${head.slice(0, 7)} (${message})`);
        return head;
      },
      verify: async (finalHead, reviewIds) => {
        checkInterrupted();
        const current = await subscriber.pull(created.fixture.repo, pr.number);
        const reviews = await subscriber.reviews(created.fixture.repo, pr.number);
        const selected = reviewIds.map(id => reviews.find(review => review.id === id));
        if (current.state !== "open" || current.merged || current.head.sha !== finalHead
          || selected.some(review => review === undefined)
          || selected[0]!.state !== "CHANGES_REQUESTED" || selected[0]!.commit_id !== reviewHeads.get(reviewIds[0]!)
          || selected[1]!.state !== "CHANGES_REQUESTED" || selected[1]!.commit_id !== reviewHeads.get(reviewIds[1]!)
          || selected[2]!.state !== "APPROVED" || selected[2]!.commit_id !== finalHead
          || await subscriber.greeting(created.fixture.repo, finalHead) !== finalLoopGreeting) {
          throw new Error("Final GitHub state did not match the scripted review loop");
        }
        collector.check();
        checkInterrupted();
      },
    });
    const run: GhRunner = args => new Promise(done => {
      execFile(join(home, "bin/gh"), args, { cwd: workDir, env, timeout: 60_000 }, (error, stdout, stderr) => {
        done({ code: error ? (typeof error.code === "number" ? error.code : null) : 0, stdout, stderr });
      });
    });
    checkInterrupted();
    const deleted = await runSubscriptionCommands(run, pr.html_url);
    console.error(`smoke: created, viewed, listed, and deleted subscription ${deleted}`);
    await save(join(options.output, "smoke.json"), { ...completed, subscriptionCommands: deleted });
  } catch (error) {
    primary = error;
    throw error;
  } finally {
    process.removeListener("SIGINT", onInterrupt);
    process.removeListener("SIGTERM", onTerminate);
    if (stream && stream.exitCode === null && stream.signalCode === null) stream.kill("SIGTERM");
    if (streamExit) {
      const stopped = await Promise.race([streamExit.then(() => true), delay(10_000).then(() => false)]).catch(() => true);
      if (!stopped && stream && stream.exitCode === null && stream.signalCode === null) {
        stream.kill("SIGKILL");
        await streamExit.catch(() => undefined);
      }
    }
    const failures: string[] = [];
    // Provisioning saves the journal before it creates anything, so read it back when provisioning failed midway.
    try { journal ??= parseJournal(JSON.parse(await readFile(journalPath, "utf8"))); }
    catch (error) {
      if (!(error instanceof Error && "code" in error && error.code === "ENOENT")) {
        failures.push(`Reading cleanup journal: ${String(error)}. Retry with: evals/cleanup ${journalPath}`);
      }
    }
    if (journal) {
      console.error("smoke: cleaning up subscriptions and sandbox repository");
      try { await cleanup(fixture, journal, journalPath, subscriber); }
      catch (error) { failures.push(`Resource cleanup: ${String(error)}. Retry with: evals/cleanup ${journalPath}`); }
    }
    try { await rm(home, { recursive: true }); await rm(workDir, { recursive: true }); }
    catch (error) { failures.push(`Local cleanup: ${String(error)}`); }
    if (failures.length) {
      const original = primary === undefined ? "" : `${primary instanceof Error ? primary.message : String(primary)}; `;
      throw new Error(original + failures.join("; "), { cause: primary });
    }
  }
  if (completed) console.log(`PASS review-loop and subscription smoke ${options.output}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main().catch(error => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
