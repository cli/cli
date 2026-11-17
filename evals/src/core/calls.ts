import { z } from "zod";

const callFields = {
  id: z.string(),
  startedAt: z.number(),
  endedAt: z.number().optional(),
  success: z.boolean().optional(),
};

export const outputSchema = z.object({
  stream: z.enum(["stdout", "stderr"]),
  at: z.number(),
  text: z.string(),
});

/** A gh invocation observed through the wrapper, or any other agent tool call observed through the SDK. */
export const callSchema = z.discriminatedUnion("kind", [
  z.object({
    ...callFields, kind: z.literal("gh"), arguments: z.array(z.string()),
    output: z.array(outputSchema), exitCode: z.number().nullable().optional(),
  }),
  z.object({ ...callFields, kind: z.literal("tool"), arguments: z.unknown() }),
]);
export type Call = z.infer<typeof callSchema>;
export type GhCall = Extract<Call, { kind: "gh" }>;

/** Flags that take a value in common gh commands; suites add their own commands' valued flags. */
export const commonValuedFlags = [
  "--repo", "-R", "--hostname", "--json", "--jq", "-q", "--template", "-t", "--method", "-X", "--field", "-F",
  "--raw-field", "-f", "--header", "-H", "--input", "--cache", "--limit", "-L",
] as const;

const fieldFlags = new Set(["-f", "-F", "--field", "--raw-field"]);

/** How to read flags that are not common to every gh command. */
export interface ArgvSpec {
  /** Flags that take a value, in addition to `commonValuedFlags`. */
  valued?: readonly string[];
  /** Alternative spellings, mapped to the canonical flag. */
  aliases?: Readonly<Record<string, string>>;
  /** Repeatable flags whose values are joined with commas rather than replaced. */
  joined?: readonly string[];
}

/** A parsed gh argv. */
export interface ParsedArgv {
  words: string[];
  /** The last value of each flag, or the comma-joined values of a `joined` flag; boolean flags are "true". */
  options: Map<string, string>;
  /** Every repeatable `gh api` field, in order. */
  fields: string[];
  help: boolean;
}

/** Parses gh argv into positional words, flag values, and `gh api` fields. */
export function parseArgv(args: string[], spec: ArgvSpec = {}): ParsedArgv {
  const valued = new Set<string>([...commonValuedFlags, ...spec.valued ?? []]);
  const joined = new Set(spec.joined ?? []);
  const words: string[] = [];
  const options = new Map<string, string>();
  const fields: string[] = [];
  const set = (flag: string, value: string) => {
    flag = spec.aliases?.[flag] ?? flag;
    const previous = options.get(flag);
    options.set(flag, joined.has(flag) && previous !== undefined ? `${previous},${value}` : value);
    if (fieldFlags.has(flag)) fields.push(value);
  };
  let help = false;
  for (let index = 0; index < args.length; index++) {
    const arg = args[index]!;
    if (arg === "--") { words.push(...args.slice(index + 1)); break; }
    if (arg === "--help" || arg === "-h") help = true;
    if (!arg.startsWith("-")) { words.push(arg); continue; }
    const short = arg.slice(0, 2);
    if (!arg.startsWith("--") && arg.length > 2 && valued.has(short)) {
      set(short, arg.slice(arg[2] === "=" ? 3 : 2));
      continue;
    }
    const equals = arg.indexOf("=");
    if (equals > 0) { set(arg.slice(0, equals), arg.slice(equals + 1)); continue; }
    if (valued.has(arg)) set(arg, args[++index] ?? "");
    else options.set(arg, "true");
  }
  return { words, options, fields, help };
}
