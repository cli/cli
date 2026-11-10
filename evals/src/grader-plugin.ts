import type { GraderRegistry } from "@microsoft/vally";
import { registerEventGraders } from "./suites/events/grading/index.js";
import { evidenceLoader } from "./suites/events/grading/evidence.js";
import { copilotReviewer } from "./graders/copilot-review.js";
import { SemanticReviewGrader } from "./graders/review.js";

/** Vally grader plugin entry point: `vally eval --grader-plugin evals/dist/grader-plugin.js`. */
export function registerGraders(registry: GraderRegistry): void {
  const facts = evidenceLoader();
  const checks = registerEventGraders(registry, facts);
  registry.register(new SemanticReviewGrader(copilotReviewer(), checks, facts));
}
