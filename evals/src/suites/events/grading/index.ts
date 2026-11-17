import type { GraderRegistry } from "@microsoft/vally";
import type { EvidenceFor } from "./evidence.js";
import { NoPollingGrader } from "./polling.js";
import { EventRouteGrader } from "./route.js";
import { EventTaskGrader } from "./task.js";

export function registerEventGraders(registry: GraderRegistry, evidenceFor: EvidenceFor) {
  const graders = [new EventTaskGrader(evidenceFor), new EventRouteGrader(evidenceFor), new NoPollingGrader(evidenceFor)] as const;
  for (const grader of graders) registry.register(grader);
  return graders;
}
