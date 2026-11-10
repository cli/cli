import { scenarioNames, type Scenario } from "./domain.js";
import { fullDelegation } from "./scenarios/full-delegation.js";
import { listenAtLimit } from "./scenarios/listen-at-limit.js";
import { notifyReview } from "./scenarios/notify-review.js";
import { pullAfterClose } from "./scenarios/pull-after-close.js";
import { pullAfterMerge } from "./scenarios/pull-after-merge.js";
import { reviewLoop } from "./scenarios/review-loop.js";
import type { ScenarioDefinition } from "./scenarios/definition.js";

export const scenarios = {
  "notify-review": notifyReview,
  "pull-after-merge": pullAfterMerge,
  "pull-after-close": pullAfterClose,
  "full-delegation": fullDelegation,
  "review-loop": reviewLoop,
  "listen-at-limit": listenAtLimit,
} satisfies Record<Scenario, ScenarioDefinition>;

/** Placeholder in eval.yml prompts; the fixture PR only exists once the suite creates it. */
export const prUrlPlaceholder = "{{PR_URL}}";

export function isScenario(value: string): value is Scenario {
  return scenarioNames.some(name => name === value);
}
