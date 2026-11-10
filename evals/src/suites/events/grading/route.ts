import type { Grader, GraderInput, GraderResult } from "@microsoft/vally";
import { dedicatedWait, subscriptionCommand } from "../commands.js";
import { analyse } from "./analysis.js";
import type { EvidenceFor } from "./evidence.js";
import { metadata, result } from "../../../graders/result.js";

/** Checks that every scripted change was observed through a dedicated event-based gh command. */
export class EventRouteGrader implements Grader {
  readonly metadata = metadata("event-route");
  constructor(private readonly evidenceFor: EvidenceFor) {}

  async grade(input: GraderInput): Promise<GraderResult> {
    if (!input.trajectory) throw new Error("Route grading requires a trajectory");
    const evidence = await this.evidenceFor(input);
    const { required, observations } = analyse(evidence);
    const graded = result(this.metadata.name, [
      [observations.length === required,
        "Each required change needs matching dedicated-command output, or a status read showing a review posted outside any listener."],
    ]);
    return {
      ...graded,
      metadata: {
        observed: observations.length,
        required,
        dedicatedWaits: new Set(observations
          .filter(observation => evidence.calls.some(call => call.id === observation.callId && dedicatedWait(call)))
          .map(observation => observation.callId)).size,
        observations,
        triedInterfaces: evidence.calls.filter(call => dedicatedWait(call)
          || (call.kind === "gh" && call.arguments.some(arg => /wait|watch|subscri|listen|^events?$/.test(arg))))
          .map(call => call.arguments),
        // Not scored: how the agent managed subscriptions, such as at the subscription limit.
        subscriptionCommands: evidence.calls.reduce<Record<string, number>>((counts, call) => {
          const name = call.kind === "gh" && call.success === true ? subscriptionCommand(call) : undefined;
          if (name) counts[name] = (counts[name] ?? 0) + 1;
          return counts;
        }, {}),
      },
    };
  }
}
