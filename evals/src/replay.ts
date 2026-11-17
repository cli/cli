import { computeMetrics, type GraderResult, type Trajectory } from "@microsoft/vally";
import { readFile, stat } from "node:fs/promises";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { evidenceFile, parseTrialEvidence } from "./suites/events/grading/evidence.js";
import { NoPollingGrader } from "./suites/events/grading/polling.js";
import { EventRouteGrader } from "./suites/events/grading/route.js";
import { EventTaskGrader } from "./suites/events/grading/task.js";

export interface ReplayResult {
  path: string;
  scenario: string;
  passed: boolean;
  graders: GraderResult[];
}

function trajectory(id: string): Trajectory {
  return {
    id, stimulus: { name: "replay", prompt: "Offline deterministic evidence replay." },
    events: [], output: "", workDir: "/unused",
    metadata: { executor: "replay", skillsLoaded: [], model: "none", sessionID: id },
    metrics: computeMetrics([]),
  };
}

export async function replay(path: string): Promise<ReplayResult> {
  const absolute = resolve(path);
  const source = (await stat(absolute)).isDirectory() ? join(absolute, evidenceFile) : absolute;
  const evidence = parseTrialEvidence(JSON.parse(await readFile(source, "utf8")));
  const load = async () => evidence;
  const input = { trajectory: trajectory(source) };
  // Raw-API stimuli ask for a direct API stream, so, as in their eval.yml, they skip the route and polling checks.
  const selected = evidence.variant === "raw-api"
    ? [new EventTaskGrader(load)]
    : [new EventTaskGrader(load), new EventRouteGrader(load), new NoPollingGrader(load)];
  const graders = await Promise.all(selected.map(grader => grader.grade(input)));
  return { path: source, scenario: evidence.scenario, passed: graders.every(result => result.passed), graders };
}

const usage = `Usage: evals/replay [--json] PATH...

Run deterministic graders against gh-event-evidence.json files or their artifact directories.
No GitHub credentials, model token, network access, or live resources are used.`;

export async function main(argv = process.argv.slice(2)): Promise<void> {
  let json = false;
  const paths: string[] = [];
  for (const arg of argv) {
    if (arg === "--help" || arg === "-h") { console.log(usage); return; }
    if (arg === "--json") json = true;
    else if (arg.startsWith("-")) throw new Error(`Unknown option: ${arg}\n${usage}`);
    else paths.push(arg);
  }
  if (!paths.length) throw new Error(usage);
  const results = await Promise.all(paths.map(replay));
  if (json) console.log(JSON.stringify(results, null, 2));
  else {
    for (const item of results) {
      console.log(`${item.passed ? "PASS" : "FAIL"} ${item.scenario} ${item.path}`);
      for (const grader of item.graders) {
        console.log(`  ${grader.passed ? "PASS" : "FAIL"} ${grader.name}: ${grader.evidence}`);
      }
    }
  }
  if (results.some(result => !result.passed)) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main().catch(error => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  });
}
