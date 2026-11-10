import {
  parseDuration, type Executor, type ExecutorOptions, type Skill, type Stimulus, type Trajectory,
} from "@microsoft/vally";
import { CopilotSdkExecutor } from "@microsoft/vally/executor";
import { appendFileSync } from "node:fs";
import { access, constants, cp, mkdir, mkdtemp, readFile, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import { ActorDriver } from "./actor.js";
import type { Call } from "./calls.js";
import { InfrastructureError } from "./errors.js";
import { evidenceFile } from "./evidence.js";
import { IsolatedClient, prepareHome } from "./home.js";
import { builtinSkillNames, inside, record, redact, save } from "./io.js";
import { CommandObserver, contamination, observe, unexpectedSkills } from "./observer.js";
import { parseJournal, type Journal } from "./sandbox.js";
import {
  cleanupSuite, parseExecutorConfig, resolvePrompt, type RunContext, type SuiteRegistry, type Trial, type TrialActor,
} from "./suite.js";
import { cleanupTrial } from "./trial-cleanup.js";

const sourceRoot = fileURLToPath(new URL("../../..", import.meta.url));

function required(name: string): string {
  const value = process.env[name];
  if (!value?.trim()) throw new Error(`${name} must be supplied explicitly`);
  return value;
}

/** The agent's working budget: a development override, the stimulus constraint, or the whole trial timeout. */
export function effectiveAgentBudget(
  override: string | undefined,
  configured: number | undefined,
  timeout: number,
): number {
  const budget = override?.trim() ? parseDuration(override) : configured ?? timeout;
  if (budget <= 0) throw new InfrastructureError("GH_EVAL_AGENT_BUDGET must be a positive duration");
  return budget;
}

/** Validates a stimulus's `tags.variant` against the variants its suite accepts. */
export function stimulusVariant(value: unknown, variants: readonly string[]): string {
  if (typeof value === "string" && variants.includes(value)) return value;
  throw new InfrastructureError(`Unsupported stimulus variant: ${String(value)}`);
}

/**
 * Vally executor that runs one live agent trial of a suite scenario: the suite provisions a fixture, the Copilot agent
 * runs in an isolated home with an observed gh, an optional second actor changes the world while the agent works, and
 * the trial's evidence is recorded for the graders. Trials never overlap.
 */
export class GhAgentExecutor implements Executor {
  readonly name = "gh-agent";
  readonly supportsMultiTurn = false;
  private context?: Promise<RunContext>;
  private queue: Promise<unknown> = Promise.resolve();
  private stopping = false;

  constructor(private readonly suites: SuiteRegistry) {}

  validateConfig(config: unknown): void {
    parseExecutorConfig(config, this.suites);
  }

  private setup(): Promise<RunContext> {
    this.context ??= (async () => {
      if (required("GH_EVAL_HOST") !== "github.com") throw new InfrastructureError("This preview suite supports github.com only");
      const org = required("GH_EVAL_ORG");
      if (!/^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$/.test(org)) throw new InfrastructureError("Invalid GH_EVAL_ORG");
      const gh = await realpath(required("GH_EVAL_GH"));
      await access(gh, constants.R_OK | constants.X_OK);
      const token = required("GH_EVAL_TOKEN");
      const configuredBudget = process.env.GH_EVAL_AGENT_BUDGET?.trim();
      const agentBudgetMs = configuredBudget ? effectiveAgentBudget(configuredBudget, undefined, 1) : undefined;
      const actorToken = process.env.GH_EVAL_ACTOR_TOKEN?.trim();
      const request: typeof fetch = fetch;
      return {
        org, gh, token, request,
        ...(actorToken ? { actorToken } : {}),
        fixtureToken: process.env.GH_EVAL_FIXTURE_TOKEN?.trim() || token,
        modelToken: required("COPILOT_GITHUB_TOKEN"),
        ...(agentBudgetMs === undefined ? {} : { agentBudgetMs }),
      };
    })();
    return this.context;
  }

  execute(stimulus: Stimulus, options: ExecutorOptions): Promise<Trajectory> {
    // Trials can share one account's limited resources, and a second actor's timing assumes it is the only change in
    // flight, so trials never overlap.
    const run = this.queue.then(() => {
      if (this.stopping) throw new InfrastructureError("The eval run was interrupted before this trial started");
      return this.trial(stimulus, options);
    });
    this.queue = run.catch(() => undefined);
    return run;
  }

  /** Vally calls this on SIGINT or SIGTERM and exits once it resolves, so wait for the running trial to clean up its
   * live resources. The trial stops its agent at its next check. */
  async shutdown(): Promise<void> {
    this.stopping = true;
    await this.queue;
  }

  private async trial(stimulus: Stimulus, options: ExecutorOptions): Promise<Trajectory> {
    const { suite, scenario } = parseExecutorConfig(options.executorConfig, this.suites);
    const context = await this.setup();
    const artifactDir = options.sessionLog?.executorArtifactsDir;
    if (!artifactDir) throw new InfrastructureError("gh-agent needs Vally's per-trial artifacts directory (run it with vally eval)");
    if (stimulus.turns?.length || stimulus.attachments?.length) {
      throw new InfrastructureError("gh-agent supports single-turn prompts without attachments");
    }
    await mkdir(artifactDir, { recursive: true, mode: 0o700 });
    const secrets = [context.token, context.actorToken ?? "", context.fixtureToken, context.modelToken].filter(Boolean);
    const log = (text: string) => appendFileSync(join(artifactDir, "runtime.log"), redact(text, secrets), { mode: 0o600 });
    const skills = options.skills ?? [];
    const label = `${stimulus.name} skill-${skills.length ? "on" : "off"}`;
    const progress = (message: string) => {
      const line = `[${new Date().toISOString().slice(11, 19)}] ${label}: ${message}`;
      console.error(redact(line, secrets));
      log(line + "\n");
    };
    const home = await mkdtemp(join(tmpdir(), "gh-work-"));
    const workDir = options.workDir;
    const journalPath = join(artifactDir, "resources.json");
    const calls: Call[] = [];
    const rawEvents: unknown[] = [];
    let observerError: Error | undefined;
    let contaminated: string | undefined;
    const observer = new CommandObserver(calls, (call, phase) => {
      const entry = phase === "output" && call.kind === "gh"
        ? { type: phase, id: call.id, chunk: call.output.at(-1) }
        : { type: phase, ...call };
      appendFileSync(join(artifactDir, "commands.jsonl"), redact(JSON.stringify(entry) + "\n", secrets), { mode: 0o600 });
      if (phase === "start" && call.kind === "gh") {
        const command = `gh ${call.arguments.join(" ")}`;
        progress(`agent ran: ${command.length > 140 ? `${command.slice(0, 137)}...` : command}`);
      }
    });
    let listening = false;
    let inner: CopilotSdkExecutor | undefined;
    let primary: unknown;
    let driver: ActorDriver<unknown> | undefined;
    try {
      for (const hidden of [sourceRoot, artifactDir]) {
        if (inside(await realpath(home), hidden) || inside(workDir, hidden)) {
          throw new InfrastructureError("Agent directories must be outside the checkout and result directories");
        }
      }
      const variant = stimulusVariant(stimulus.tags?.variant, suite.variants);
      const env = await prepareHome(home, context);
      await observer.listen(join(home, ".commands.sock"));
      listening = true;
      const staged: Skill[] = [];
      for (const skill of skills) {
        if (!skill.path) throw new InfrastructureError(`Skill ${skill.name} has no path to stage`);
        const target = join(home, ".skills", basename(dirname(skill.path)));
        await cp(dirname(skill.path), target, { recursive: true, dereference: false });
        staged.push({ ...skill, path: join(target, basename(skill.path)) });
      }
      const skillSource = process.env.GH_EVAL_SKILL_SOURCE?.trim();
      if (staged.length && skillSource) log(`gh skill: ${skillSource}\n`);
      const budget = context.agentBudgetMs ?? effectiveAgentBudget(undefined, options.maxAgentDurationMs, options.timeout);
      const trial: Trial = await suite.start({
        run: context, scenario, workDir, env, journalPath, agentBudgetMs: budget, progress,
      });
      progress(`fixture ready: ${trial.summary}`);
      const actor: TrialActor | undefined = trial.actor;
      const drive = actor ? new ActorDriver(actor, async actions => {
        for (const action of actions) progress(`second actor: ${actor.describe(action)}`);
        await save(join(artifactDir, "actor.json"), actor.actions, secrets);
      }) : undefined;
      driver = drive;
      const trigger = () => actor?.trigger(calls, Date.now());
      const resolved: Stimulus = { ...stimulus, prompt: resolvePrompt(stimulus.prompt, trial.placeholders) };
      const disabled = builtinSkillNames();
      const client = () => new IsolatedClient(disabled, {
        workingDirectory: workDir, baseDirectory: env.COPILOT_HOME, env,
        useLoggedInUser: false, gitHubToken: context.modelToken,
      }, drive && actor?.postTool ? () => drive.tick(trigger(), "post-tool") : undefined);
      inner = new CopilotSdkExecutor({ createClient: client, createEnvClient: client });
      let finished = false;
      let finishedAt = 0;
      let executionError: unknown;
      let trajectory: Trajectory | undefined;
      progress(`agent started (${options.model ?? "default model"}, ${Math.round(budget / 1000)}s budget, skill ${skills.length ? "on" : "off"})`);
      const execution = inner.execute(resolved, {
        ...options, maxAgentDurationMs: budget, skills: staged, env: undefined,
        onRawEvent: event => {
          options.onRawEvent?.(event);
          rawEvents.push(event);
          appendFileSync(join(artifactDir, "events.jsonl"), redact(JSON.stringify(event) + "\n", secrets), { mode: 0o600 });
          const type = record(event).type;
          if (typeof type === "string" && /^(tool\.execution_|assistant\.message|user\.message|session\.skills_loaded)/.test(type)) {
            contaminated ??= contamination(event, [sourceRoot]);
          }
          try { observe(calls, event, Date.now()); }
          catch (error) { observerError = error instanceof Error ? error : new Error(String(error)); }
        },
      }).then(value => { trajectory = value; }, error => { executionError = error; })
        .finally(() => { finished = true; finishedAt = Date.now(); });
      let lastTick = 0;
      let lastReadyId: string | undefined;
      let lastLoggedId: string | undefined;
      while (!finished) {
        if (this.stopping) throw new InfrastructureError("The eval run was interrupted");
        if (observerError) throw observerError;
        if (observer.error) throw observer.error;
        if (drive?.error) throw drive.error;
        if (contaminated) throw new InfrastructureError(`Agent reached hidden material: ${contaminated}`);
        const ready = trigger();
        if (ready && ready.id !== lastLoggedId) {
          progress(actor!.triggerNote);
          lastLoggedId = ready.id;
        }
        if (drive && actor && !actor.done && (ready?.id !== lastReadyId || Date.now() - lastTick >= 5_000)) {
          await drive.tick(ready);
          lastTick = Date.now();
          lastReadyId = ready?.id;
        }
        await delay(100);
      }
      await execution;
      await drive?.close();
      progress(executionError ? "agent errored" : `agent finished${trajectory?.endReason ? ` (${trajectory.endReason})` : ""}`);
      if (observerError) throw observerError;
      if (observer.error) throw observer.error;
      if (contaminated) throw new InfrastructureError(`Agent reached hidden material: ${contaminated}`);
      if (executionError) throw executionError;
      if (!trajectory) throw new InfrastructureError("The Copilot executor produced no trajectory");
      trial.validate?.(rawEvents);
      const unexpected = unexpectedSkills(rawEvents, skills.map(skill => skill.name));
      if (unexpected.length) {
        throw new InfrastructureError(`Unexpected skills were available to the agent (${unexpected.join(", ")})`);
      }
      const snapshot = await trial.snapshot(finishedAt);
      await save(join(artifactDir, evidenceFile), {
        version: 3, scenario, variant, skill: skills.length > 0, calls, snapshot,
      }, secrets);
      return { ...trajectory, artifactDir };
    } catch (error) {
      primary = error;
      throw error;
    } finally {
      await cleanupTrial({
        async actor() { await driver?.close(); },
        async agent() { await inner?.shutdown(); },
        async observer() { if (listening) await observer.close(); },
        async resources() {
          let journal: Journal;
          try { journal = parseJournal(JSON.parse(await readFile(journalPath, "utf8"))); }
          catch (error) {
            if (error instanceof Error && "code" in error && error.code === "ENOENT") return;
            throw new Error(`Reading cleanup journal: ${String(error)}`, { cause: error });
          }
          progress("cleaning up live resources");
          await cleanupSuite(suite, journal, journalPath, context);
          progress("cleanup complete");
        },
        async local() { await rm(home, { recursive: true }); },
      }, primary, journalPath);
    }
  }
}
