interface CleanupActions {
  actor(): Promise<void>;
  agent(): Promise<void>;
  observer(): Promise<void>;
  resources(): Promise<void>;
  local(): Promise<void>;
}

class TrialCleanupError extends Error {
  readonly retryable = false;
}

/** Attempts every cleanup step while retaining the original execution failure. */
export async function cleanupTrial(actions: CleanupActions, primary: unknown, journalPath: string): Promise<void> {
  const failures: string[] = [];
  let resourceFailure = false;
  for (const [step, label] of [
    ["actor", "Second actor execution"],
    ["agent", "Copilot executor shutdown"],
    ["observer", "Command observer cleanup"],
    ["resources", "Resource cleanup"],
    ["local", "Local cleanup"],
  ] as const) {
    try { await actions[step](); }
    catch (error) {
      if (step === "actor" && primary !== undefined && error === primary) continue;
      failures.push(`${label}: ${String(error)}`);
      if (step === "resources") resourceFailure = true;
    }
  }
  if (failures.length) {
    const original = primary === undefined ? "" : `${primary instanceof Error ? primary.message : String(primary)}; `;
    const retry = resourceFailure ? `. Retry with: evals/cleanup ${journalPath}` : "";
    throw new TrialCleanupError(`${original}${failures.join("; ")}${retry}`, { cause: primary });
  }
}
