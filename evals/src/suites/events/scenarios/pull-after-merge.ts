import { mergedGreeting } from "../domain.js";
import type { ScenarioDefinition } from "./definition.js";

export const pullAfterMerge = {
  fixture: { greeting: mergedGreeting, pullRequest: "existing" },
  steps: [{ kind: "merge" }],
  taskChecks({ snapshot }) {
    return [
      [snapshot.pr?.merged === true && snapshot.actions.some(action => action.kind === "merge"),
        "The scripted merge did not happen."],
      [snapshot.mainContainsMerge && snapshot.currentBranch === "main", "Local main does not contain the merged change."],
      [snapshot.localGreeting === mergedGreeting, "The expected merged file is missing from the working tree."],
    ];
  },
} satisfies ScenarioDefinition;
