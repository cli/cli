import { proposedGreeting, reviewedGreeting } from "../domain.js";
import type { ScenarioDefinition } from "./definition.js";

export const finalLoopGreeting = "Hello, GitHub!\n";

export const reviewLoop = {
  fixture: { greeting: proposedGreeting, pullRequest: "existing", author: "agent" },
  steps: [
    {
      kind: "review", event: "REQUEST_CHANGES",
      delivery: "after-tool",
      body: "Please add an exclamation mark to greeting.txt so its complete line is: Hello, world!",
    },
    {
      kind: "review", event: "REQUEST_CHANGES", afterFix: reviewedGreeting,
      body: "One more change: replace world with GitHub in greeting.txt so its complete line is: Hello, GitHub!",
    },
    {
      kind: "approval", greeting: finalLoopGreeting, delivery: "after-fix",
      body: "Both requested changes are now correct. Approved.",
    },
  ],
  taskChecks({ snapshot }) {
    const [first, second] = snapshot.actions.filter(action => action.kind === "review");
    const approval = snapshot.actions.find(action => action.kind === "approval");
    const firstFix = snapshot.actions.findLast(action => action.kind === "fix" && action.head === second?.head
      && first !== undefined && second !== undefined && first.at <= action.at && action.at <= second.at);
    const secondFix = snapshot.actions.findLast(action => action.kind === "fix" && action.head === approval?.head
      && second !== undefined && approval !== undefined && second.at <= action.at && action.at <= approval.at);
    const ordered = first && firstFix && second && secondFix && approval
      && first.at <= firstFix.at && firstFix.at <= second.at
      && second.at <= secondFix.at && secondFix.at <= approval.at;
    const reviews = [first, second, approval];
    return [
      [Boolean(ordered), "Two requested-change rounds, each followed by a verified pushed fix, must precede approval."],
      [Boolean(firstFix && secondFix && first && second && approval
        && firstFix.head !== first.head && second.head === firstFix.head
        && secondFix.head !== second.head && approval.head === secondFix.head),
      "Each review round must be addressed on a distinct pushed head."],
      [reviews.every((action, index) => action?.reviewId !== undefined
        && snapshot.reviews.some(review => review.id === action.reviewId && review.commit_id === action.head
          && review.state === (index === 2 ? "APPROVED" : "CHANGES_REQUESTED")))
        && new Set(reviews.map(action => action?.reviewId)).size === 3,
      "Three distinct submitted reviews must match the scripted states and reviewed commits."],
      [Boolean(approval && snapshot.pr?.head.sha === approval.head), "Approval must apply to the final PR head."],
      [snapshot.greeting === finalLoopGreeting, "The final pushed file must contain both requested changes."],
      [snapshot.pr?.state === "open" && snapshot.pr.merged === false, "Leave the approved PR open; do not merge it."],
    ];
  },
} satisfies ScenarioDefinition;
