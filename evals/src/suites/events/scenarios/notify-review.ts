import { mergedGreeting } from "../domain.js";
import type { ScenarioDefinition } from "./definition.js";

export const notifyReview = {
  fixture: { greeting: mergedGreeting, pullRequest: "existing" },
  steps: [{
    kind: "review",
    event: "COMMENT",
    body: "I've reviewed this change. The greeting looks good.",
  }],
  taskChecks({ snapshot }) {
    return [
      [snapshot.actions.some(action => action.kind === "review"
        && snapshot.reviews.some(review => review.id === action.reviewId)),
      "The scripted review was not found on GitHub."],
    ];
  },
} satisfies ScenarioDefinition;
