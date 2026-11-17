import type { Grader, GraderInput, GraderResult } from "@microsoft/vally";
import { analyse } from "./analysis.js";
import type { EvidenceFor } from "./evidence.js";
import { metadata, result } from "../../../graders/result.js";

/**
 * Reports whether the agent avoided polling, independently of whether it completed the task.
 * Reducing polling load is the point of the Events API, so it is scored on its own.
 */
export class NoPollingGrader implements Grader {
  readonly metadata = metadata("no-polling");
  constructor(private readonly evidenceFor: EvidenceFor) {}

  async grade(input: GraderInput): Promise<GraderResult> {
    if (!input.trajectory) throw new Error("Polling grading requires a trajectory");
    const evidence = await this.evidenceFor(input);
    const { polling, firstWait, statusReads } = analyse(evidence);
    const afterWait = polling.filter(call => firstWait !== undefined && call.startedAt >= firstWait);
    const graded = result(this.metadata.name, [
      [!evidence.skill || polling.length === 0, "With the gh skill available, the agent repeated status reads to wait."],
      [evidence.skill || polling.length === 0 || firstWait !== undefined,
        "Without the skill, the agent polled and never switched to a dedicated event-based wait."],
      [afterWait.length === 0, "The agent polled after it had started using event-based waiting."],
    ]);
    return {
      ...graded,
      metadata: {
        pollingReads: polling.length,
        pollingCallIds: polling.map(call => call.id),
        statusReads,
        ghInvocations: evidence.calls.filter(call => call.kind === "gh").length,
      },
    };
  }
}
