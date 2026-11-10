import { z } from "zod";

export const pullRequestSchema = z.looseObject({
  number: z.number(),
  html_url: z.string(),
  state: z.string(),
  merged: z.boolean(),
  mergeable: z.boolean().nullable(),
  merge_commit_sha: z.string().nullable(),
  head: z.looseObject({ sha: z.string(), ref: z.string() }),
  base: z.looseObject({ ref: z.string() }),
});
export type PullRequest = z.infer<typeof pullRequestSchema>;

export const reviewSchema = z.looseObject({
  id: z.number(),
  state: z.string(),
  commit_id: z.string(),
});
export type Review = z.infer<typeof reviewSchema>;

/** A GitHub REST API error response. */
export class ApiError extends Error {
  constructor(readonly status: number, method: string, path: string, message: string, readonly retryAfterMs?: number) {
    super(`GitHub ${method} ${path}: HTTP ${status}: ${message}`);
  }
}

function transportCode(error: unknown): string | undefined {
  for (const candidate of [error, error instanceof Error ? error.cause : undefined]) {
    if (typeof candidate === "object" && candidate !== null && "code" in candidate && typeof candidate.code === "string") {
      return candidate.code;
    }
  }
}

/** A minimal GitHub REST client for fixtures, actors, and snapshots, with retries for transient failures. */
export class GitHub {
  constructor(
    private readonly token: string,
    private readonly request: typeof fetch = fetch,
    private readonly retryDelaysMs: number[] = [2_000, 5_000, 10_000, 20_000],
  ) {}

  async api<T>(method: string, path: string, body?: unknown): Promise<T> {
    return (await this.page<T>(method, path, body)).body;
  }

  /** Whether an API error is transient for this route beyond the generic idempotent-retry rule. Subclasses add
   * rules for APIs that document extra retryable responses. */
  protected transient(_method: string, _path: string, _status: number): boolean {
    return false;
  }

  /** Sends a request like `api` and also returns the relative path of the next page from the Link header. */
  protected async page<T>(method: string, path: string, body?: unknown): Promise<{ body: T; next?: string }> {
    if (!path.startsWith("/") || path.startsWith("//")) throw new Error("Expected a relative GitHub API path");
    const idempotent = ["GET", "PUT", "PATCH", "DELETE"].includes(method);
    for (let attempt = 0; ; attempt++) {
      try {
        return await this.once<T>(method, path, body);
      } catch (error) {
        const code = transportCode(error);
        const reset = method === "GET" && code === "ECONNRESET";
        // The REST API intermittently answers with 5xx or 429. Repeating a GET, PUT, PATCH or DELETE leaves the
        // same end state, so retrying keeps a transient outage from erroring a whole trial. POSTs (reviews, repos,
        // pull requests) are not retried: a 5xx may follow a completed write and a retry could duplicate it.
        const retryableStatus = error instanceof ApiError && (
          (idempotent && [429, 500, 502, 503, 504].includes(error.status))
          || this.transient(method, path, error.status));
        const attempts = reset ? Math.min(1, this.retryDelaysMs.length) : this.retryDelaysMs.length;
        if ((!reset && !retryableStatus) || attempt >= attempts) {
          if (error instanceof ApiError) throw error;
          const detail = error instanceof Error ? error.message : String(error);
          const suffix = code && !detail.includes(code) ? ` (${code})` : "";
          throw new Error(`GitHub ${method} ${path}: ${detail}${suffix}`, { cause: error });
        }
        if (reset) console.error(`GitHub ${method} ${path}: connection reset; retrying GET once.`);
        const delay = Math.min((error instanceof ApiError ? error.retryAfterMs : undefined) ?? this.retryDelaysMs[attempt]!, 30_000);
        await new Promise(resolve => setTimeout(resolve, delay));
      }
    }
  }

  private async once<T>(method: string, path: string, body?: unknown): Promise<{ body: T; next?: string }> {
    const response = await this.request(`https://api.github.com${path}`, {
      method,
      redirect: "error",
      signal: AbortSignal.timeout(30_000),
      headers: {
        Authorization: `Bearer ${this.token}`,
        Accept: "application/vnd.github+json",
        "Content-Type": "application/json",
        "User-Agent": "gh-evals",
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    const text = await response.text();
    if (!response.ok) {
      const retryAfterHeader = response.headers.get("retry-after");
      const retryAfter = retryAfterHeader === null ? Number.NaN : Number(retryAfterHeader);
      throw new ApiError(response.status, method, path, text || response.statusText,
        Number.isFinite(retryAfter) && retryAfter >= 0 ? retryAfter * 1000 : undefined);
    }
    const next = /<https:\/\/api\.github\.com(\/[^>]*)>;\s*rel="next"/.exec(response.headers.get("link") ?? "")?.[1];
    return { body: (text ? JSON.parse(text) : undefined) as T, ...(next ? { next } : {}) };
  }

  pull(repo: string, number: number): Promise<PullRequest> {
    return this.api("GET", `/repos/${repo}/pulls/${number}`);
  }

  async reviews(repo: string, number: number): Promise<Review[]> {
    const reviews: Review[] = [];
    for (let page = 1; ; page++) {
      const batch = await this.api<Review[]>("GET", `/repos/${repo}/pulls/${number}/reviews?per_page=100&page=${page}`);
      if (!Array.isArray(batch)) throw new Error("Expected a reviews array");
      reviews.push(...batch);
      if (batch.length < 100) return reviews;
    }
  }

  /** Reads a text file from a repository at a ref. */
  async fileContent(repo: string, path: string, ref: string): Promise<string> {
    const file = await this.api<{ encoding: string; content: string }>(
      "GET", `/repos/${repo}/contents/${path}?ref=${encodeURIComponent(ref)}`,
    );
    if (file.encoding !== "base64") throw new Error(`Unexpected ${path} encoding`);
    return Buffer.from(file.content, "base64").toString("utf8");
  }
}
