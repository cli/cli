import { execFile } from "node:child_process";
import { readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { promisify } from "node:util";
import { mkdir, writeFile, rename } from "node:fs/promises";
import { dirname, relative, isAbsolute, join } from "node:path";

const exec = promisify(execFile);

export function record(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("Expected an object");
  }
  return value as Record<string, unknown>;
}

/**
 * Names of the skills bundled with the Copilot runtime. They load whenever skill directories are
 * configured, so skill-on trials would otherwise get these extras (including the PR-related
 * github-pr-media) while skill-off trials do not. They also expose a path inside the checkout.
 */
export function builtinSkillNames(): string[] {
  try {
    const manifest = createRequire(import.meta.url)
      .resolve(`@github/copilot-sdk-${process.platform}-${process.arch}/package.json`);
    return readdirSync(join(dirname(manifest), "builtin"), { withFileTypes: true })
      .filter(entry => entry.isDirectory()).map(entry => entry.name).sort();
  } catch (error) {
    if (error instanceof Error && "code" in error
      && (error.code === "MODULE_NOT_FOUND" || error.code === "ENOENT")) return [];
    throw error;
  }
}

export function inside(path: string, root: string): boolean {
  const rel = relative(root, path);
  return rel === "" || (!isAbsolute(rel) && rel !== ".." && !rel.startsWith(`..${process.platform === "win32" ? "\\" : "/"}`));
}

export function isolatedEnvironment(home: string, path: string, modelToken: string): NodeJS.ProcessEnv {
  return {
    HOME: home,
    USERPROFILE: home,
    TMPDIR: join(home, "tmp"),
    XDG_CONFIG_HOME: join(home, ".config"),
    XDG_CACHE_HOME: join(home, ".cache"),
    XDG_DATA_HOME: join(home, ".local/share"),
    GH_CONFIG_DIR: join(home, ".config/gh"),
    COPILOT_HOME: join(home, ".copilot"),
    COPILOT_CLI: "1",
    COPILOT_GITHUB_TOKEN: modelToken,
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_CONFIG_GLOBAL: join(home, ".gitconfig"),
    GIT_TERMINAL_PROMPT: "0",
    GCM_INTERACTIVE: "never",
    PATH: path,
    LANG: "en_US.UTF-8",
    TERM: "dumb",
    NO_COLOR: "1",
  };
}

export async function git(workDir: string, env: NodeJS.ProcessEnv, ...args: string[]): Promise<string> {
  const { stdout } = await exec("git", args, { cwd: workDir, env, timeout: 30_000, maxBuffer: 2 * 1024 * 1024 });
  return stdout.trim();
}

export async function containsCommit(workDir: string, env: NodeJS.ProcessEnv, sha: string): Promise<boolean> {
  return (await git(workDir, env, "rev-list", "refs/heads/main")).split("\n").includes(sha);
}

export function redact(text: string, secrets: string[]): string {
  for (const secret of secrets.filter(Boolean)) {
    for (const form of [secret, encodeURIComponent(secret), Buffer.from(secret).toString("base64")]) {
      text = text.replaceAll(form, "[REDACTED]");
    }
  }
  return text;
}

export async function save(path: string, value: unknown, secrets: string[] = []): Promise<void> {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 });
  const temporary = `${path}.tmp`;
  await writeFile(temporary, redact(JSON.stringify(value, null, 2) + "\n", secrets), { mode: 0o600 });
  await rename(temporary, path);
}
