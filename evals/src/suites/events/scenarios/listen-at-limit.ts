import { mergedGreeting } from "../domain.js";
import type { ScenarioDefinition } from "./definition.js";

export const listenAtLimit = {
  fixture: { greeting: mergedGreeting, pullRequest: "existing", seedSubscriptions: true },
  // Every slot is taken, so the agent has to free one before it can listen. Which subscriptions it deletes is
  // reported by the task grader but not scored.
  steps: [{
    kind: "review",
    event: "COMMENT",
    body: "I've reviewed this change. The greeting looks good.",
  }],
  taskChecks({ snapshot }) {
    return [
      [snapshot.subscriptions !== undefined, "The seeded subscriptions were not recorded."],
      [snapshot.actions.some(action => action.kind === "review"
        && snapshot.reviews.some(review => review.id === action.reviewId)),
      "The scripted review was not found on GitHub."],
    ];
  },
} satisfies ScenarioDefinition;
