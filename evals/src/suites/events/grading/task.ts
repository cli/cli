import type { Grader, GraderInput, GraderResult } from "@microsoft/vally";
import { scenarios } from "../scenarios.js";
import type { EvidenceFor } from "./evidence.js";
import { metadata, result } from "../../../graders/result.js";

export class EventTaskGrader implements Grader {
  readonly metadata = metadata("event-task");
  constructor(private readonly evidenceFor: EvidenceFor) {}

  async grade(input: GraderInput): Promise<GraderResult> {
    if (!input.trajectory) throw new Error("Task grading requires a trajectory");
    const evidence = await this.evidenceFor(input);
    const graded = result(this.metadata.name, scenarios[evidence.scenario].taskChecks(evidence));
    const seeded = evidence.snapshot.subscriptions;
    if (!seeded) return graded;
    const remaining = new Set(seeded.remaining);
    return {
      ...graded,
      metadata: {
        staleDeleted: seeded.stale.filter(id => !remaining.has(id)).length,
        activeDeleted: seeded.active.filter(id => !remaining.has(id)).length,
        stale: seeded.stale.length,
        active: seeded.active.length,
      },
    };
  }
}
