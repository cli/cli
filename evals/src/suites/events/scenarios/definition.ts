import type { Evidence } from "../grading/evidence.js";
import type { Check } from "../../../graders/result.js";

export type ActorStep =
  | {
    kind: "review";
    event: "COMMENT" | "REQUEST_CHANGES";
    body: string;
    afterFix?: string;
    delivery?: "during-wait" | "after-tool";
  }
  | { kind: "approval"; greeting: string; body: string; delivery?: "during-wait" | "after-fix" }
  | { kind: "merge" }
  | { kind: "close" };

export interface ScenarioDefinition {
  fixture: {
    greeting: string;
    pullRequest: "existing" | "agent-created";
    author?: "agent" | "fixture";
    /** Fill the agent's subscription limit before the trial with subscriptions on other fixture pull requests, some
     * closed and some open, so the agent has to free a slot before it can listen. */
    seedSubscriptions?: boolean;
  };
  steps: readonly [ActorStep, ...ActorStep[]];
  taskChecks(evidence: Evidence): Check[];
}
