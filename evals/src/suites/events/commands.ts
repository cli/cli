import { parseArgv, type Call, type ParsedArgv } from "../../core/calls.js";

/** How long a dedicated wait has been attached before the second actor reacts, so the agent can re-read like it would
 * against a real reviewer, who takes far longer than this. */
export const actorReactionDelayMs = 2_000;

const argvSpec = {
  valued: ["--timeout", "--resource", "--event", "--events", "--subscription", "--count"],
  aliases: { "--events": "--event" },
  joined: ["--event"],
} as const;

/** Parses gh argv for event commands. `--event` values are joined with commas, including its earlier `--events`
 * spelling. */
export function command(args: string[]): ParsedArgv {
  return parseArgv(args, argvSpec);
}

export function dedicatedWait(call: Call): "stream" | undefined {
  if (call.kind !== "gh") return;
  const { words, help } = command(call.arguments);
  if (help) return;
  if (words[0] === "event" && words[1] === "listen") return "stream";
}

/** The `gh event subscription` subcommand a call ran, or "api" for a `gh api` call on the subscriptions API. Help
 * requests are not counted. */
export function subscriptionCommand(call: Call): "list" | "view" | "create" | "delete" | "api" | undefined {
  if (call.kind !== "gh") return;
  const { words, help } = command(call.arguments);
  if (help) return;
  if (words[0] === "event" && (words[1] === "subscription" || words[1] === "subscriptions")) {
    const sub = words[2] === "ls" ? "list" : words[2];
    if (sub === "list" || sub === "view" || sub === "create" || sub === "delete") return sub;
  }
  if (words[0] === "api" && /^\/?event-subscriptions(?:[/?]|$)/.test(words[1] ?? "")) return "api";
}

/** Whether a listener keeps listening after its first event, with `--follow` or a `--count` above 1. */
export function persistentWait(call: Call): boolean {
  if (call.kind !== "gh" || !dedicatedWait(call)) return false;
  const { options } = command(call.arguments);
  return options.get("--follow") === "true" || Number(options.get("--count") ?? 1) > 1;
}

/** The times a listener reported on stderr that its stream attached, and whether it still is: it reports each
 * reconnect before attaching again. Changes made while no stream is attached are never delivered. */
export function attachments(call: Call): { at: number[]; attached: boolean } {
  const at: number[] = [];
  let attached = false;
  if (call.kind !== "gh") return { at, attached };
  let partial = "";
  for (const chunk of call.output) {
    if (chunk.stream !== "stderr") continue;
    const lines = (partial + chunk.text).split("\n");
    partial = lines.pop() ?? "";
    for (const line of lines) {
      if (line.startsWith("Listening for events")) {
        at.push(chunk.at);
        attached = true;
      } else if (/reconnecting|retrying|creating it again/.test(line)) {
        attached = false;
      }
    }
  }
  return { at, attached };
}

/** When the listener's current stream attached, or undefined while no stream is attached. */
export function attachedSince(call: Call): number | undefined {
  const { at, attached } = attachments(call);
  return attached ? at.at(-1) : undefined;
}

/** How long the second actor gives the agent, once a listener attaches, to finish a catch-up status read. */
export const actorCatchUpLimitMs = 15_000;

/** Whether the second actor may act during `wait`. The stream must have been attached for the reaction delay, and the
 * agent must have finished a status read started after the attachment, or had the catch-up limit to do so. Acting
 * sooner lets that read reveal the change before the listener can deliver it, which a real reviewer rarely causes. */
export function readyForActor(wait: Call, calls: Call[], now: number): boolean {
  const attached = attachedSince(wait);
  if (attached === undefined || now - attached < actorReactionDelayMs) return false;
  return now - attached >= actorCatchUpLimitMs || calls.some(call => call.startedAt >= attached
    && call.endedAt !== undefined && statusRead(call) !== undefined);
}

/** Repeated successful reads of the same projection between events count as polling. */
export function statusRead(call: Call): string | undefined {
  if (call.kind !== "gh" || call.success !== true) return;
  const { words, options, fields, help } = command(call.arguments);
  if (help) return;
  if (words[0] === "pr" && words[1] === "view") {
    const fields = options.get("--json")?.split(",").sort();
    if (fields && !fields.some(field => ["reviews", "latestReviews", "reviewDecision", "state", "closed", "closedAt", "mergedAt"].includes(field))) return;
    return JSON.stringify(["pr-view", words.slice(2), options.get("--repo") ?? options.get("-R"), fields]);
  }
  if (words[0] === "api") {
    const method = options.get("--method") ?? options.get("-X")
      ?? (["-f", "-F", "--field", "--raw-field", "--input"].some(flag => options.has(flag)) ? "POST" : "GET");
    const path = words[1];
    if (method.toUpperCase() === "GET" && path && /(?:^|\/)pulls\/\d+(?:[/?]|$)/.test(path)) {
      return `api:${path}`;
    }
    const query = fields.find(field => field.startsWith("query="));
    if (path === "graphql" && query?.includes("pullRequest") && !/\bmutation\b/.test(query)) {
      // The bindings of the query's variables tell apart reads of different pull requests with the same query.
      const variables = new Set(Array.from(query.matchAll(/\$(\w+)/g), match => match[1]));
      const bindings = fields.filter(field => field !== query && variables.has(field.split("=")[0])).sort();
      return JSON.stringify(["graphql", query, bindings]);
    }
  }
}
