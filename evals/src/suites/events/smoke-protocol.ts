export interface SmokeProtocol {
  currentHead(): Promise<string>;
  submitReview(head: string, event: "REQUEST_CHANGES" | "APPROVE", body: string): Promise<number>;
  waitForReview(id: number): Promise<void>;
  pushGreeting(greeting: string, message: string): Promise<string>;
  verify(finalHead: string, reviewIds: readonly number[]): Promise<void>;
}

export interface SmokeSummary {
  initialHead: string;
  firstFix: string;
  finalHead: string;
  reviews: [number, number, number];
}

/** Exercises the complete stream-driven review loop without any model decisions. */
export async function runSmokeProtocol(protocol: SmokeProtocol): Promise<SmokeSummary> {
  const initialHead = await protocol.currentHead();
  const first = await protocol.submitReview(initialHead, "REQUEST_CHANGES",
    "Please add an exclamation mark to greeting.txt so its complete line is: Hello, world!");
  await protocol.waitForReview(first);
  const firstFix = await protocol.pushGreeting("Hello, world!\n", "Complete greeting punctuation");
  const second = await protocol.submitReview(firstFix, "REQUEST_CHANGES",
    "One more change: replace world with GitHub in greeting.txt so its complete line is: Hello, GitHub!");
  await protocol.waitForReview(second);
  const finalHead = await protocol.pushGreeting("Hello, GitHub!\n", "Use GitHub in greeting");
  const approval = await protocol.submitReview(finalHead, "APPROVE", "Both requested changes are now correct. Approved.");
  await protocol.waitForReview(approval);
  const reviews: [number, number, number] = [first, second, approval];
  await protocol.verify(finalHead, reviews);
  return { initialHead, firstFix, finalHead, reviews };
}

/** Runs the built gh with the given arguments, without a terminal, and returns how it exited. */
export type GhRunner = (args: string[]) => Promise<{ code: number | null; stdout: string; stderr: string }>;

/**
 * Exercises every `gh event subscription` command against a live pull request: create a subscription the listener
 * does not use, see it through view and list, delete it, and confirm it is gone. Returns the deleted subscription ID.
 */
export async function runSubscriptionCommands(gh: GhRunner, prUrl: string): Promise<string> {
  const expect = async (args: string[]) => {
    const result = await gh(args);
    if (result.code !== 0) throw new Error(`gh ${args.join(" ")} exited ${result.code}: ${result.stderr.trim()}`);
    return result.stdout;
  };
  const type = "com.github.pull_request.merge_conflict.v1";
  const id = (await expect(["event", "subscription", "create", "--resource", prUrl, "--event", type])).trim();
  if (!/^sub_[A-Za-z0-9]+$/.test(id)) throw new Error(`gh event subscription create printed no subscription ID: ${id}`);
  const viewed = JSON.parse(await expect(["event", "subscription", "view", id, "--json", "id,resourceUrl,eventTypes"]));
  if (viewed.id !== id || viewed.resourceUrl !== prUrl || JSON.stringify(viewed.eventTypes) !== JSON.stringify([type])) {
    throw new Error(`gh event subscription view returned an unexpected subscription: ${JSON.stringify(viewed)}`);
  }
  const listed: Array<{ id: string }> = JSON.parse(await expect(["event", "subscription", "list", "--limit", "100", "--json", "id"]));
  if (!listed.some(subscription => subscription.id === id)) throw new Error(`gh event subscription list omitted ${id}`);
  await expect(["event", "subscription", "delete", id]);
  const gone = await gh(["event", "subscription", "view", id]);
  if (gone.code === 0 || !gone.stderr.includes("was not found")) {
    throw new Error(`gh event subscription view still found ${id} after deletion: ${gone.stderr.trim()}`);
  }
  return id;
}

export class ReviewEventCollector {
  private text = "";
  private readonly received = new Set<number>();
  private readonly waiting = new Map<number, Array<() => void>>();
  private readonly failing = new Set<(error: Error) => void>();
  private failure: Error | undefined;

  /** Records stream output. Malformed output fails the current and later waits instead of throwing, because this
   * runs in a stream callback, where a throw would skip the smoke's cleanup. */
  write(chunk: string): void {
    if (this.failure) return;
    this.text += chunk;
    for (let newline = this.text.indexOf("\n"); newline >= 0; newline = this.text.indexOf("\n")) {
      const line = this.text.slice(0, newline);
      this.text = this.text.slice(newline + 1);
      if (!line.trim()) continue;
      let event: unknown;
      try { event = JSON.parse(line); }
      catch (error) {
        this.failure = new Error(`Event stream printed a line that is not JSON: ${line.slice(0, 200)}`, { cause: error });
        for (const reject of this.failing) reject(this.failure);
        this.failing.clear();
        return;
      }
      if (typeof event !== "object" || event === null) continue;
      const data = (event as { data?: { api_url?: unknown } }).data;
      const match = typeof data?.api_url === "string" ? /\/reviews\/(\d+)$/.exec(data.api_url) : null;
      if (!match) continue;
      const id = Number(match[1]);
      this.received.add(id);
      for (const resolve of this.waiting.get(id) ?? []) resolve();
      this.waiting.delete(id);
    }

  }

  async wait(id: number, timeoutMs: number): Promise<void> {
    this.check();
    if (this.received.has(id)) return;
    let resolve!: () => void;
    const observed = new Promise<void>(done => { resolve = done; });
    const listeners = this.waiting.get(id) ?? [];
    listeners.push(resolve);
    this.waiting.set(id, listeners);
    const timeout = new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error(`Timed out waiting for review event ${id}`)), timeoutMs).unref();
    });
    let reject!: (error: Error) => void;
    const failed = new Promise<never>((_, fail) => { reject = fail; });
    this.failing.add(reject);
    try { await Promise.race([observed, timeout, failed]); }
    finally { this.failing.delete(reject); }
    this.check();
  }

  /** Throws if the stream has printed malformed output, including output after the review that was awaited. */
  check(): void {
    if (this.failure) throw this.failure;
  }
}

export class StreamReadySignal {
  private text = "";
  private ready = false;
  private readonly waiters: Array<() => void> = [];

  write(chunk: string): void {
    this.text += chunk;
    if (!this.ready && this.text.includes("Listening for events.\n")) {
      this.ready = true;
      for (const resolve of this.waiters) resolve();
      this.waiters.length = 0;
    }
  }

  async wait(timeoutMs: number): Promise<void> {
    if (this.ready) return;
    const observed = new Promise<void>(resolve => this.waiters.push(resolve));
    const timeout = new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error("Timed out waiting for the event stream to attach")), timeoutMs).unref();
    });
    await Promise.race([observed, timeout]);
  }
}
