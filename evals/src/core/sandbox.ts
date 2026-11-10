import { randomUUID } from "node:crypto";
import type { Dirent } from "node:fs";
import { readFile, readdir } from "node:fs/promises";
import { join } from "node:path";
import { z } from "zod";
import { ApiError, type GitHub } from "./github.js";
import { save } from "./io.js";

const journalV1Schema = z.object({
  version: z.literal(1),
  repo: z.string(),
  id: z.number().optional(),
  cleaned: z.boolean(),
  deleting: z.boolean().optional(),
  subscriptions: z.array(z.string()),
});

const journalSchema = z.object({
  version: z.literal(2),
  /** The suite whose cleanup removes the trial's other resources. */
  suite: z.string(),
  repo: z.string(),
  id: z.number().optional(),
  cleaned: z.boolean(),
  /** Set once the repository's deletion was requested, so a later 404 confirms it. */
  deleting: z.boolean().optional(),
  /** IDs of suite-specific resources by kind, recorded before they can leak. */
  resources: z.record(z.string(), z.array(z.string())),
});

/** The crash-safe record of a trial's live resources, written before each one is created. */
export type Journal = z.infer<typeof journalSchema>;

/** Validates a journal, reading the first version as an events-suite journal. */
export function parseJournal(value: unknown): Journal {
  const parsed = journalSchema.safeParse(value);
  if (parsed.success) return parsed.data;
  const v1 = journalV1Schema.safeParse(value);
  if (!v1.success) throw new Error("Invalid resource journal");
  const { subscriptions, ...rest } = v1.data;
  return { ...rest, version: 2, suite: "events", resources: { subscriptions } };
}

/** Reads and validates a journal file. */
export const parseJournalFile = async (path: string): Promise<Journal> =>
  parseJournal(JSON.parse(await readFile(path, "utf8")));

/** Records a suite resource in the journal and persists it. */
export async function journalResource(journal: Journal, journalPath: string, kind: string, id: string): Promise<void> {
  journal.resources[kind] = [...new Set([...journal.resources[kind] ?? [], id])];
  await save(journalPath, journal);
}

const sandboxName = /^[A-Za-z0-9-]+\/run-[0-9a-f-]{36}$/;

/** Creates an empty private sandbox repository in `org`, journaling it before and after creation. */
export async function createSandbox(
  github: GitHub, org: string, suite: string, journalPath: string,
): Promise<{ repo: string; journal: Journal }> {
  const repo = `${org}/run-${randomUUID()}`;
  const journal: Journal = { version: 2, suite, repo, cleaned: false, resources: {} };
  await save(journalPath, journal);
  let created: { id: number; full_name: string; private: boolean };
  try {
    created = await github.api("POST", `/orgs/${org}/repos`, {
      name: repo.split("/")[1], private: true, has_issues: false, has_wiki: false,
      allow_merge_commit: true, auto_init: false,
    });
  } catch (error) {
    if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
      journal.cleaned = true;
      await save(journalPath, journal);
    }
    throw error;
  }
  journal.id = created.id;
  await save(journalPath, journal);
  if (created.full_name.toLowerCase() !== repo.toLowerCase() || !created.private) {
    throw new Error("GitHub created an unexpected repository");
  }
  return { repo, journal };
}

/**
 * Deletes the sandbox repository after confirming its identity and privacy. `beforeDelete` removes the suite's other
 * resources while the repository still exists, so they can be matched to it.
 */
export async function cleanupSandbox(
  github: GitHub, journal: Journal, journalPath: string, beforeDelete?: () => Promise<void>,
): Promise<void> {
  if (journal.cleaned) return;
  if (!sandboxName.test(journal.repo)) throw new Error("Refusing cleanup of an invalid journal repository");
  let repo: { id: number; private: boolean };
  try {
    repo = await github.api("GET", `/repos/${journal.repo}`);
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 404) throw error;
    if (!journal.deleting) throw new Error("Fixture repository is no longer accessible; cleanup cannot be confirmed");
    journal.cleaned = true;
    await save(journalPath, journal);
    return;
  }
  if (!repo.private || (journal.id !== undefined && journal.id !== repo.id)) {
    throw new Error("Refusing cleanup: repository identity or privacy changed");
  }
  await beforeDelete?.();
  // Other resources are gone and the repository exists, so a later 404 means this deletion went through, even when
  // its response was lost or the process stopped before recording it.
  journal.deleting = true;
  await save(journalPath, journal);
  try {
    await github.api("DELETE", `/repos/${journal.repo}`);
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 404) throw error;
  }
  journal.cleaned = true;
  await save(journalPath, journal);
}

/** Finds resource journals under a results root whose cleanup has not completed. */
export async function unfinishedJournals(root: string): Promise<string[]> {
  // Vally nests each trial as <run>/<eval>/<stimulus>/<model>/<trial>/executor-artifacts/, so walk
  // the tree rather than assume a depth. The cap stops a mistaken root from walking a whole disk.
  const found: string[] = [];
  const visit = async (directory: string, depth: number): Promise<void> => {
    let entries: Dirent[];
    try { entries = await readdir(directory, { withFileTypes: true }); }
    catch (error) {
      if (error instanceof Error && "code" in error && (error.code === "ENOENT" || error.code === "ENOTDIR")) return;
      throw error;
    }
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
      const path = join(directory, entry.name);
      if (entry.isDirectory() && depth < 10) await visit(path, depth + 1);
      else if (entry.isFile() && entry.name === "resources.json") {
        try {
          if (!(await parseJournalFile(path)).cleaned) found.push(path);
        } catch {
          found.push(path);
        }
      }
    }
  };
  await visit(root, 0);
  return found;
}
