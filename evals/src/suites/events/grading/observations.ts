import type { Action } from "../domain.js";
import type { GhCall } from "../../../core/calls.js";
import { command, dedicatedWait } from "../commands.js";
import type { Evidence } from "./evidence.js";
import { record } from "../../../core/io.js";

function object(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value) ? record(value) : undefined;
}

function json(text: string): unknown {
  try { return JSON.parse(text); } catch { return undefined; }
}

export function jsonOutput(call: GhCall): Array<{ at: number; value: unknown }> {
  const chunks = call.output.filter(chunk => chunk.stream === "stdout");
  const whole = json(chunks.map(chunk => chunk.text).join(""));
  if (whole !== undefined && chunks.length) return [{ at: chunks.at(-1)!.at, value: whole }];
  const values: Array<{ at: number; value: unknown }> = [];
  let pending = "";
  for (const chunk of chunks) {
    pending += chunk.text;
    let newline: number;
    while ((newline = pending.indexOf("\n")) >= 0) {
      const value = json(pending.slice(0, newline));
      if (value !== undefined) values.push({ at: chunk.at, value });
      pending = pending.slice(newline + 1);
    }
  }
  if (pending.trim() && chunks.length) {
    const value = json(pending);
    if (value !== undefined) values.push({ at: chunks.at(-1)!.at, value });
  }
  return values;
}

function containsReview(value: unknown, reviewId: number, expected?: Record<string, unknown>): boolean {
  if (Array.isArray(value)) return value.some(item => containsReview(item, reviewId, expected));
  const item = object(value);
  if (!item) return false;
  if (item.id === reviewId || item.fullDatabaseId === reviewId) return true;
  const commit = object(item.commit)?.oid ?? item.commit_id;
  if (expected !== undefined && item.state === expected.state && item.body === expected.body
    && commit === expected.commit_id) return true;
  return Object.values(item).some(nested => containsReview(nested, reviewId, expected));
}

function words(text: string): string {
  return text.split(/\s+/).filter(Boolean).join(" ");
}

/** Returns when a status read showed the exact scripted review, either as JSON or, for rendered output such as
 * `gh pr view --comments`, as the review's body text. Each scripted review has a distinct body. */
export function reviewReadAt(call: GhCall, action: Action, expected?: Record<string, unknown>): number | undefined {
  if ((action.kind !== "review" && action.kind !== "approval") || action.reviewId === undefined) return;
  for (const output of jsonOutput(call)) {
    if (containsReview(output.value, action.reviewId, expected)) return output.at;
  }
  const body = typeof expected?.body === "string" ? words(expected.body) : "";
  if (!body) return;
  let text = "";
  for (const chunk of call.output) {
    if (chunk.stream !== "stdout") continue;
    text += chunk.text;
    if (words(text).includes(body)) return chunk.at;
  }
}

const eventTypes = {
  review: "com.github.pull_request.review.submitted.v1",
  approval: "com.github.pull_request.review.submitted.v1",
  merge: "com.github.pull_request.state.changed.v1",
  close: "com.github.pull_request.state.changed.v1",
} as const;

/** Whether a --resource value names the fixture PR, by web URL or by either REST API URL form the server accepts. */
function namesPull(resource: string, pull: NonNullable<Evidence["snapshot"]["pr"]>): boolean {
  if (resource === pull.html_url) return true;
  const [owner, repo] = new URL(pull.html_url).pathname.split("/").filter(Boolean);
  if (resource === `https://api.github.com/repos/${owner}/${repo}/pulls/${pull.number}`) return true;
  const repositoryId = object(object(pull.base)?.repo)?.id;
  return typeof repositoryId === "number"
    && resource === `https://api.github.com/repositories/${repositoryId}/pulls/${pull.number}`;
}

function eventMatchesPull(call: GhCall, pull: Evidence["snapshot"]["pr"], apiUrl: string, action: Action): boolean {
  if (!pull) return false;
  const parsed = command(call.arguments);
  const resource = parsed.options.get("--resource");
  if (resource !== undefined && !namesPull(resource, pull)) return false;
  const web = new URL(pull.html_url);
  const [owner, repo] = web.pathname.split("/").filter(Boolean);
  const named = `https://api.github.com/repos/${owner}/${repo}/pulls/${action.pr}`;
  const baseRepo = object(pull.base)?.repo;
  const repositoryId = object(baseRepo)?.id;
  const numeric = typeof repositoryId === "number"
    ? `https://api.github.com/repositories/${repositoryId}/pulls/${action.pr}`
    : undefined;
  const numericFromBoundResource = resource !== undefined
    && new RegExp(`^https://api\\.github\\.com/repositories/\\d+/pulls/${action.pr}(?:/|$)`).test(apiUrl);
  const prefix = apiUrl === named || apiUrl.startsWith(`${named}/`)
    || numericFromBoundResource
    || (numeric !== undefined && (apiUrl === numeric || apiUrl.startsWith(`${numeric}/`)));
  if (!prefix) return false;
  if ((action.kind === "review" || action.kind === "approval") && action.reviewId !== undefined) {
    return apiUrl.endsWith(`/reviews/${action.reviewId}`);
  }
  return true;
}

const eventAliases: Record<string, string> = {
  review: eventTypes.review,
  state: eventTypes.merge,
};

/**
 * Output reshaped by --jq or --template cannot be matched to an event, so any output counts when the listener was
 * bound to the fixture PR and the event type of the change. Output after the next change of the same event type may
 * belong to that change instead, so it does not count.
 */
function filteredOutputAt(evidence: Evidence, action: Action, call: GhCall, inWindow: (at: number) => boolean) {
  if (action.kind === "fix") return;
  const { options } = command(call.arguments);
  if (!["--jq", "-q", "--template", "-t"].some(flag => options.has(flag))) return;
  const pull = evidence.snapshot.pr;
  const resource = options.get("--resource");
  if (!pull || resource === undefined || !namesPull(resource, pull)) return;
  const events = (options.get("--event") ?? "").split(",").map(name => eventAliases[name.trim()] ?? name.trim());
  const type = eventTypes[action.kind];
  if (!events.includes(type)) return;
  const next = Math.min(...evidence.snapshot.actions
    .filter(other => other.kind !== "fix" && eventTypes[other.kind] === type && other.at > action.at)
    .map(other => other.at));
  return call.output.find(chunk => chunk.stream === "stdout" && chunk.text.trim() && inWindow(chunk.at)
    && chunk.at < next)?.at;
}

/** Requires output for the scripted change on the fixture PR, not just overlapping execution. */
export function observedAt(evidence: Evidence, action: Action, call: GhCall): number | undefined {
  if (!dedicatedWait(call) || action.kind === "fix" || !evidence.snapshot.pr) return;
  const pull = evidence.snapshot.pr;
  const inWindow = (at: number) => at >= action.at && at <= evidence.snapshot.observedUntil
    && (call.endedAt === undefined || at <= call.endedAt);
  for (const output of jsonOutput(call)) {
    if (!inWindow(output.at)) continue;
    const value = object(output.value);
    if (!value) continue;
    const apiUrl = object(value.data)?.api_url;
    if (value.type === eventTypes[action.kind] && typeof value.id === "string" && value.id.length
      && typeof apiUrl === "string" && eventMatchesPull(call, pull, apiUrl, action)) return output.at;
  }
  return filteredOutputAt(evidence, action, call, inWindow);
}
