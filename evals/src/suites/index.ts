import type { SuiteRegistry } from "../core/suite.js";
import { eventsSuite } from "./events/suite.js";

/** Every eval suite the gh-agent executor can run, by the name eval.yml files use in `config.suite`. */
export const suites: SuiteRegistry = { events: eventsSuite };
