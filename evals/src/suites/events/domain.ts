import { z } from "zod";
import { pullRequestSchema, reviewSchema, type PullRequest } from "../../core/github.js";

export const scenarioNames = ["notify-review", "pull-after-merge", "pull-after-close", "full-delegation", "review-loop",
  "listen-at-limit"] as const;
export type Scenario = typeof scenarioNames[number];

export const originalGreeting = "Hello\n";
export const proposedGreeting = "Hello, world\n";
export const reviewedGreeting = "Hello, world!\n";
export const mergedGreeting = "Hello from the merged change\n";

export interface Fixture {
  scenario: Scenario;
  repo: string;
  workDir: string;
  baseSha: string;
  initialHead: string;
  pr?: PullRequest;
  seeded?: SeededSubscriptions;
}

export const seededSubscriptionsSchema = z.object({
  /** Seeded on pull requests the fixture closed. */
  stale: z.array(z.string()),
  /** Seeded on other open pull requests. */
  active: z.array(z.string()),
});
export type SeededSubscriptions = z.infer<typeof seededSubscriptionsSchema>;

export const actionSchema = z.object({
  kind: z.enum(["review", "fix", "approval", "merge", "close"]),
  at: z.number(),
  pr: z.number(),
  head: z.string(),
  trigger: z.enum(["wait", "fallback", "verification", "after-tool", "after-fix"]),
  callId: z.string().optional(),
  reviewId: z.number().optional(),
});
export type Action = z.infer<typeof actionSchema>;

export const snapshotSchema = z.object({
  observedUntil: z.number(),
  pr: pullRequestSchema.optional(),
  greeting: z.string().optional(),
  localGreeting: z.string().optional(),
  mainContainsMerge: z.boolean(),
  currentBranch: z.string(),
  actions: z.array(actionSchema),
  reviews: z.array(reviewSchema),
  /** The subscriptions the fixture seeded to fill the limit, and which of them remained when the agent finished. */
  subscriptions: seededSubscriptionsSchema.extend({ remaining: z.array(z.string()) }).optional(),
});
export type Snapshot = z.infer<typeof snapshotSchema>;
