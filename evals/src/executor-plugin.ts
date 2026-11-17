import type { ExecutorRegistry } from "@microsoft/vally";
import { GhAgentExecutor } from "./core/executor.js";
import { suites } from "./suites/index.js";

/** Vally executor plugin entry point: `vally eval --executor-plugin evals/dist/executor-plugin.js`. */
export function registerExecutors(registry: ExecutorRegistry): void {
  registry.register(new GhAgentExecutor(suites));
}
