import type { GraderMetadata, GraderResult } from "@microsoft/vally";

export type Check = readonly [passed: boolean, failure: string];

export function metadata(name: string): GraderMetadata {
  return {
    name, description: `Checks ${name} deterministically using recorded commands, output, and fixture state.`,
    behavior: {}, determinism: "complex-static", reference: "reference-free",
    temporalScope: "trajectory-level", costProfile: "free",
  };
}

export function result(name: string, checks: Check[]): GraderResult {
  const failures = checks.filter(([passed]) => !passed).map(([, reason]) => reason);
  return {
    name, kind: "code", passed: failures.length === 0, score: failures.length ? 0 : 1,
    evidence: failures.length ? failures.join("; ") : "All required observations were verified.",
  };
}
