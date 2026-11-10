import { mergedGreeting, originalGreeting } from "../domain.js";
import type { ScenarioDefinition } from "./definition.js";

export const pullAfterClose = {
  fixture: { greeting: mergedGreeting, pullRequest: "existing" },
  steps: [{ kind: "close" }],
  taskChecks({ snapshot }) {
    return [
      [snapshot.pr?.state === "closed" && snapshot.pr.merged === false, "The PR was not closed without merging."],
      [snapshot.actions.some(action => action.kind === "close"), "The scripted close did not happen."],
      [snapshot.currentBranch === "main" && !snapshot.mainContainsMerge, "The local main branch changed unexpectedly."],
      [snapshot.localGreeting === originalGreeting, "The local working file changed despite the PR not merging."],
    ];
  },
} satisfies ScenarioDefinition;
