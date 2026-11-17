import { attachments, dedicatedWait, statusRead } from "../commands.js";
import type { Call } from "../../../core/calls.js";
import { scenarios } from "../scenarios.js";
import type { Evidence } from "./evidence.js";
import { observedAt, reviewReadAt } from "./observations.js";

export interface Observation {
  callId: string;
  startedAt: number;
  at: number;
  action: string;
  recovered: boolean;
}

export interface RouteAnalysis {
  required: number;
  observations: Observation[];
  firstWait?: number;
  polling: Call[];
  statusReads: number;
}

/** Mechanical facts shared by the route and polling graders, computed once from recorded evidence. */
export function analyse(evidence: Evidence): RouteAnalysis {
  const { scenario, calls, snapshot } = evidence;
  const changes = snapshot.actions.filter(action => action.kind !== "fix");
  const observations = changes.flatMap((action, index): Observation[] => {
    const step = scenarios[scenario].steps[index];
    if (!step || step.kind !== action.kind) return [];
    const delivery = "delivery" in step ? step.delivery : undefined;
    const recovery = delivery === "after-tool" || delivery === "after-fix";
    const trigger = delivery === "after-tool" ? "after-tool" : delivery === "after-fix" ? "after-fix" : "wait";
    // Recovery steps are graded on outcome, so a late fallback delivery is held to the same evidence.
    if (action.trigger !== trigger && !(recovery && action.trigger === "fallback")) return [];
    // The actor acts during one listener, but an agent may run several bound to different events, so any listener
    // running at the change may deliver it. The actor's own listener is credited first.
    const candidates = [...calls].sort((a, b) => Number(b.id === action.callId) - Number(a.id === action.callId));
    for (const call of candidates) {
      if (call.kind !== "gh" || (!recovery && call.startedAt > action.at)
        || (call.endedAt ?? snapshot.observedUntil) < action.at) continue;
      const at = observedAt(evidence, action, call);
      if (at !== undefined) {
        return [{ callId: call.id, startedAt: call.startedAt, at, action: action.kind, recovered: call.startedAt > action.at }];
      }
    }
    if (recovery) {
      // A review posted outside any listener is graded on outcome: any status read that shows it proves nothing
      // was missed, whether the read ran before, alongside or after a listener.
      const expected = snapshot.reviews.find(review => review.id === action.reviewId);
      for (const read of calls) {
        if (read.kind !== "gh" || !statusRead(read)) continue;
        const at = reviewReadAt(read, action, expected);
        if (at !== undefined && at >= action.at) {
          return [{ callId: read.id, startedAt: read.startedAt, at, action: action.kind, recovered: true }];
        }
      }
    }
    return [];
  });
  const waitCalls = calls.filter(call => dedicatedWait(call));
  const waits = waitCalls.map(call => call.startedAt);
  // A status read is polling when it repeats the same projection with no intervening external change or
  // listener boundary. Actor changes, listener starts, stream attachments and listener exits each open a new
  // epoch, so one read per change is allowed, as are the reads the hint and the listener ask for: one once a
  // stream is attached, including after each reconnect, to catch changes made while none was, and one after the
  // listener exits, to see what a thin event changed.
  const boundaries = [
    ...changes.map(change => change.at),
    ...waits,
    ...waitCalls.flatMap(call => attachments(call).at),
    ...waitCalls.flatMap(call => call.endedAt === undefined ? [] : [call.endedAt]),
  ];
  const reads = new Set<string>();
  let statusReads = 0;
  const polling = [...calls].sort((a, b) => a.startedAt - b.startedAt).filter(call => {
    const projection = statusRead(call);
    if (!projection) return false;
    statusReads++;
    const epoch = boundaries.filter(at => at <= call.startedAt).length;
    const key = `${epoch}:${projection}`;
    if (reads.has(key)) return true;
    reads.add(key);
    return false;
  });
  return {
    required: scenarios[scenario].steps.length,
    observations: changes.length === scenarios[scenario].steps.length ? observations : [],
    firstWait: waits.length ? Math.min(...waits) : undefined,
    polling,
    statusReads,
  };
}
