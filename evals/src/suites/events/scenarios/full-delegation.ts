import { proposedGreeting, reviewedGreeting } from "../domain.js";
import type { ScenarioDefinition } from "./definition.js";

export const fullDelegation = {
  fixture: { greeting: proposedGreeting, pullRequest: "agent-created" },
  steps: [
    {
      kind: "review",
      event: "REQUEST_CHANGES",
      body: "Please add an exclamation mark to greeting.txt so its complete line is: Hello, world!",
    },
    { kind: "approval", greeting: reviewedGreeting, body: "Thanks for fixing the greeting. Approved." },
  ],
  taskChecks({ snapshot }) {
    const review = snapshot.actions.find(action => action.kind === "review");
    const approval = snapshot.actions.find(action => action.kind === "approval");
    const fix = snapshot.actions.findLast(action => action.kind === "fix" && action.head === approval?.head);
    const approvedReview = snapshot.reviews.find(review => review.id === approval?.reviewId);
    return [
      [Boolean(review && fix && approval && review.at <= fix.at && fix.at <= approval.at),
        "A requested change, verified pushed fix, and subsequent approval are required."],
      [snapshot.pr?.merged === true, "The agent did not merge the PR."],
      [Boolean(approvedReview?.state === "APPROVED" && approvedReview.commit_id === approval?.head),
        "The approval of the fixed head was not found on GitHub."],
      [Boolean(approval && snapshot.pr?.head.sha === approval.head), "The merged head was not the approved head."],
      [snapshot.greeting === reviewedGreeting, "The merged greeting is not the reviewed fix."],
    ];
  },
} satisfies ScenarioDefinition;
