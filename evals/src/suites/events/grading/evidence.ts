import type { GraderInput, Trajectory } from "@microsoft/vally";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { z } from "zod";
import { callSchema } from "../../../core/calls.js";
import { evidenceFile } from "../../../core/evidence.js";
import { scenarioNames, snapshotSchema } from "../domain.js";

export { evidenceFile };

const evidenceBody = {
  scenario: z.enum(scenarioNames),
  skill: z.boolean(),
  calls: z.array(callSchema),
  snapshot: snapshotSchema,
};

const trialEvidenceV2Schema = z.object({
  version: z.literal(2),
  ...evidenceBody,
});

const trialEvidenceSchema = z.object({
  version: z.literal(3),
  variant: z.enum(["eval", "design-probe", "raw-api", "unknown"]),
  ...evidenceBody,
});

export type TrialEvidence = z.infer<typeof trialEvidenceSchema>;
export type Evidence = TrialEvidence;
export type EvidenceFor = (input: GraderInput) => Promise<Evidence>;

export function parseTrialEvidence(value: unknown): TrialEvidence {
  const current = trialEvidenceSchema.safeParse(value);
  if (current.success) return current.data;
  const previous = trialEvidenceV2Schema.safeParse(value);
  if (previous.success) {
    return {
      ...previous.data,
      version: 3,
      variant: "unknown",
    };
  }
  throw new Error(`Malformed ${evidenceFile}: ${current.error.message}`);
}

export async function readTrialEvidence(trajectory: Trajectory): Promise<TrialEvidence> {
  if (!trajectory.artifactDir) {
    throw new Error("No artifact directory on the trajectory; these graders need a trial run by the gh-agent executor");
  }

  const value: unknown = JSON.parse(await readFile(join(trajectory.artifactDir, evidenceFile), "utf8"));
  return parseTrialEvidence(value);
}

export function evidenceLoader(): EvidenceFor {
  const cache = new Map<string, Promise<Evidence>>();
  return input => {
    if (!input.trajectory) return Promise.reject(new Error("Grading requires a trajectory"));
    let pending = cache.get(input.trajectory.id);
    if (!pending) {
      const id = input.trajectory.id;
      pending = readTrialEvidence(input.trajectory);
      pending.catch(() => cache.delete(id));
      cache.set(id, pending);
    }
    return pending;
  };
}
