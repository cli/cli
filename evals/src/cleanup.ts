import { realpath } from "node:fs/promises";
import { pathToFileURL } from "node:url";
import { resolve } from "node:path";
import { redact } from "./core/io.js";
import { parseJournalFile, unfinishedJournals } from "./core/sandbox.js";
import { cleanupSuite } from "./core/suite.js";
import { suites } from "./suites/index.js";

const usage = `Usage:
  evals/cleanup <resources.json>   Delete one trial's leftover resources and sandbox repository
  evals/cleanup --unfinished <dir> List trials under a results directory whose cleanup did not finish`;

/** Housekeeping outside vally eval: retrying a trial's cleanup, and finding trials that need it. */
export async function main(argv = process.argv.slice(2)): Promise<void> {
  if (argv[0] === "--unfinished" && argv[1]) {
    const journals = await unfinishedJournals(resolve(argv[1]));
    if (!journals.length) return;
    console.error(`${journals.length} trial(s) have unfinished cleanup. Leftover resources can count toward account `
      + "limits (event subscriptions do for 24 hours). If no other run is using them, clean up with:");
    for (const path of journals) console.error(`  evals/cleanup ${path}`);
    return;
  }
  if (argv.length !== 1 || argv[0]!.startsWith("-")) throw new Error(usage);
  const token = process.env.GH_EVAL_TOKEN;
  if (!token?.trim()) throw new Error("GH_EVAL_TOKEN must be the same user token the trial ran with");
  const path = await realpath(argv[0]!);
  const request: typeof fetch = fetch;
  const journal = await parseJournalFile(path);
  const suite = Object.hasOwn(suites, journal.suite) ? suites[journal.suite] : undefined;
  if (!suite) throw new Error(`The journal names an unknown suite: ${journal.suite}`);
  await cleanupSuite(suite, journal, path, { token, fixtureToken: process.env.GH_EVAL_FIXTURE_TOKEN || token, request });
  console.log("Resource cleanup complete.");
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main().catch(error => {
    console.error(redact(error instanceof Error ? error.message : String(error), [process.env.GH_EVAL_TOKEN ?? ""]));
    process.exitCode = 1;
  });
}
