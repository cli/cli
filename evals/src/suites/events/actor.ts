import { persistentWait } from "./commands.js";
import { EventsGitHub } from "./github.js";
import type { Action, Fixture } from "./domain.js";
import type { Call } from "../../core/calls.js";
import type { PullRequest } from "../../core/github.js";
import { scenarios } from "./scenarios.js";
import type { ActorStep } from "./scenarios/definition.js";

export type Phase = ActorStep["kind"];

export class Actor {
  readonly actions: Action[] = [];
  private step: ActorStep;
  private readonly remaining: ActorStep[];
  private phaseStartedAt: number;
  private verifiedHead?: string;
  private phaseHead: string;
  private fallbackStartedAt: number;
  done = false;

  constructor(
    private readonly github: EventsGitHub,
    private readonly reviewer: EventsGitHub,
    readonly fixture: Fixture,
    private readonly fallbackMs: number,
    now: number,
  ) {
    const [first, ...remaining] = scenarios[fixture.scenario].steps;
    this.step = first;
    this.remaining = remaining;
    this.phaseStartedAt = now;
    this.fallbackStartedAt = now;
    this.phaseHead = fixture.initialHead;
  }

  get phase(): Phase {
    return this.step.kind;
  }

  async tick(now: number, wait?: Call, source: "timer" | "post-tool" = "timer"): Promise<void> {
    if (this.done) return;
    const { fixture } = this;
    if (!fixture.pr) {
      const pulls = await this.github.api<Array<{ number: number }>>(
        "GET", `/repos/${fixture.repo}/pulls?state=all&base=main&per_page=100`,
      );
      if (!pulls.length) return;
      if (pulls.length !== 1 || !pulls[0]) throw new Error("Expected exactly one fixture PR");
      fixture.pr = await this.github.pull(fixture.repo, pulls[0].number);
    }
    const pr = await this.github.pull(fixture.repo, fixture.pr.number);
    fixture.pr = pr;
    if (pr.state !== "open") {
      this.done = true;
      return;
    }
    if (this.step.kind === "merge" && pr.mergeable !== true) {
      if (pr.mergeable === false) throw new Error("The fixture PR has an unexpected merge conflict");
      return;
    }
    const expectedFix = this.step.kind === "approval" ? this.step.greeting
      : this.step.kind === "review" ? this.step.afterFix : undefined;
    if (expectedFix !== undefined && this.verifiedHead !== pr.head.sha) {
      if (pr.head.sha === this.phaseHead) return;
      if (await this.github.greeting(fixture.repo, pr.head.sha) !== expectedFix) return;
      this.actions.push({ kind: "fix", at: now, pr: pr.number, head: pr.head.sha, trigger: "verification" });
      this.verifiedHead = pr.head.sha;
      this.fallbackStartedAt = now;
    }
    // A listener started before this phase only counts if it keeps listening after an event: a one-shot listener
    // from an earlier phase may be about to exit with that phase's event.
    const phaseWait = wait && wait.endedAt === undefined
      && wait.startedAt <= now
      && (wait.startedAt >= this.phaseStartedAt || persistentWait(wait)
        || this.actions.some(action => action.callId === wait.id));
    const afterTool = this.step.kind === "review" && this.step.delivery === "after-tool" && source === "post-tool";
    // The fix may only become visible after the push's post-tool tick, and an agent that then blocks on a listener
    // completes no further tool, so an attached listener can take the approval once the fix is verified.
    const afterFix = this.step.kind === "approval" && this.step.delivery === "after-fix"
      && (source === "post-tool" || Boolean(phaseWait));
    const requiresPostTool = (this.step.kind === "review" && this.step.delivery === "after-tool")
      || (this.step.kind === "approval" && this.step.delivery === "after-fix");
    const duringWait = !requiresPostTool && phaseWait;
    if (!afterTool && !afterFix && !duringWait && now - this.fallbackStartedAt < this.fallbackMs) return;
    const viaListener = duringWait || (afterFix && source !== "post-tool");
    const action: Action = {
      kind: this.phase, at: now, pr: pr.number, head: pr.head.sha,
      trigger: afterTool ? "after-tool" : afterFix ? "after-fix" : duringWait ? "wait" : "fallback",
      ...(viaListener && wait ? { callId: wait.id } : {}),
    };
    const reviewId = await this.perform(this.step, pr);
    if (reviewId !== undefined) action.reviewId = reviewId;
    this.actions.push(action);
    const next = this.remaining.shift();
    if (next) {
      this.step = next;
      this.phaseStartedAt = action.at;
      this.fallbackStartedAt = action.at;
      this.phaseHead = action.head;
      this.verifiedHead = undefined;
    } else {
      this.done = true;
    }

  }

  private async perform(step: ActorStep, pr: PullRequest): Promise<number | undefined> {
    const path = `/repos/${this.fixture.repo}/pulls/${pr.number}`;
    switch (step.kind) {
      case "review":
      case "approval": {
        const review = await this.reviewer.api<{ id: number }>("POST", `${path}/reviews`, {
          commit_id: pr.head.sha,
          event: step.kind === "review" ? step.event : "APPROVE",
          body: step.body,
        });
        return review.id;
      }
      case "merge": {
        const merged = await this.reviewer.api<{ merged: boolean }>("PUT", `${path}/merge`, {
          sha: pr.head.sha, merge_method: "merge",
        });
        if (!merged.merged) throw new Error("The scripted merge was not accepted");
        return;
      }
      case "close":
        await this.reviewer.api("PATCH", path, { state: "closed" });
        return;
      default:
        throw new Error(`Unsupported actor step: ${JSON.stringify(step satisfies never)}`);
    }
  }
}
