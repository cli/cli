import { fileURLToPath } from "node:url";
import type { Call } from "./calls.js";
import { GitHub } from "./github.js";
import { cleanupSandbox, type Journal } from "./sandbox.js";
import { record } from "./io.js";

/** Credentials and settings shared by every trial of a run, all supplied explicitly through the environment. */
export interface RunContext {
  org: string;
  /** The already-built gh binary the agent uses. */
  gh: string;
  /** The agent's GitHub user token. */
  token: string;
  /** The second actor's token, when supplied. */
  actorToken?: string;
  /** The token that provisions fixtures; defaults to the agent's. */
  fixtureToken: string;
  /** The token for the Copilot model. */
  modelToken: string;
  request: typeof fetch;
  agentBudgetMs?: number;
}

/** What a suite gets to set up one trial. */
export interface TrialContext {
  run: RunContext;
  scenario: string;
  /** The agent's working directory. */
  workDir: string;
  /** The agent's isolated environment, for git commands on its working directory. */
  env: NodeJS.ProcessEnv;
  /** Where the suite journals live resources before creating them; see `createSandbox`. */
  journalPath: string;
  agentBudgetMs: number;
  progress(message: string): void;
}

/** A scripted second actor that changes the world while the agent works. */
export interface TrialActor<A = unknown> {
  readonly done: boolean;
  readonly actions: readonly A[];
  /** Whether the actor also ticks after each successful tool, before its result returns to the agent. */
  readonly postTool: boolean;
  /** The running call the actor may act during, if any. */
  trigger(calls: readonly Call[], now: number): Call | undefined;
  /** The progress line logged when a new trigger call is recognised. */
  readonly triggerNote: string;
  tick(now: number, trigger: Call | undefined, source: "timer" | "post-tool"): Promise<void>;
  /** A one-line progress message for an action the actor took. */
  describe(action: A): string;
}

/** One provisioned trial of a suite scenario. */
export interface Trial {
  /** Values for `{{NAME}}` placeholders in stimulus prompts. */
  placeholders: Readonly<Record<string, string>>;
  /** A progress line describing the ready fixture. */
  summary: string;
  actor?: TrialActor;
  /** Throws an `InfrastructureError` when shared state, not the agent, makes the run ungradable. */
  validate?(rawEvents: readonly unknown[]): void;
  /** The suite-specific facts graders need, captured once the agent has finished. */
  snapshot(observedUntil: number): Promise<unknown>;
}

/** Credentials for cleaning up a trial outside the executor. */
export interface CleanupCredentials {
  token: string;
  fixtureToken: string;
  request: typeof fetch;
}

/** A family of scenarios sharing fixtures, an actor, and graders. */
export interface Suite {
  readonly name: string;
  readonly scenarios: readonly string[];
  /** Stimulus `tags.variant` values the suite accepts. */
  readonly variants: readonly string[];
  start(context: TrialContext): Promise<Trial>;
  /**
   * Removes every live resource the journal records, including the sandbox repository. Suites that create nothing
   * beyond the sandbox can omit it; `cleanupSuite` then deletes the sandbox with the fixture token.
   */
  cleanup?(journal: Journal, journalPath: string, credentials: CleanupCredentials): Promise<void>;
}

/** Runs a suite's cleanup, or deletes only its sandbox repository when the suite defines none. */
export function cleanupSuite(
  suite: Suite, journal: Journal, journalPath: string, credentials: CleanupCredentials,
): Promise<void> {
  if (suite.cleanup) return suite.cleanup(journal, journalPath, credentials);
  return cleanupSandbox(new GitHub(credentials.fixtureToken, credentials.request), journal, journalPath);
}

/** Suites by name. */
export type SuiteRegistry = Readonly<Record<string, Suite>>;

/** The `gh-agent` executor config: which suite scenario a stimulus runs. */
export interface ExecutorConfig {
  suite: Suite;
  scenario: string;
}

/** Parses and resolves `defaults.executor.config` for the gh-agent executor. */
export function parseExecutorConfig(value: unknown, suites: SuiteRegistry): ExecutorConfig {
  const config = record(value);
  const unknown = Object.keys(config).filter(key => key !== "suite" && key !== "scenario");
  if (unknown.length) throw new Error(`gh-agent config has unknown field(s): ${unknown.join(", ")}`);
  const suite = typeof config.suite === "string" && Object.hasOwn(suites, config.suite) ? suites[config.suite] : undefined;
  if (!suite) throw new Error(`gh-agent config.suite must be one of: ${Object.keys(suites).join(", ")}`);
  if (typeof config.scenario !== "string" || !suite.scenarios.includes(config.scenario)) {
    throw new Error(`gh-agent config.scenario must be one of: ${suite.scenarios.join(", ")}`);
  }
  return { suite, scenario: config.scenario };
}

/** Substitutes `{{NAME}}` placeholders, refusing prompts that need a value the fixture does not provide. */
export function resolvePrompt(prompt: string, placeholders: Readonly<Record<string, string>>): string {
  return prompt.replaceAll(/\{\{([A-Z][A-Z0-9_]*)\}\}/g, (match, name: string) => {
    const value = Object.hasOwn(placeholders, name) ? placeholders[name] : undefined;
    if (value === undefined) throw new Error(`The prompt references ${match}, but this fixture does not provide it`);
    return value;
  });
}

/** The checked-in eval definition of a suite scenario. */
export function definitionPath(suite: string, scenario: string): string {
  return fileURLToPath(new URL(`../../suites/${suite}/${scenario}/eval.yml`, import.meta.url));
}
