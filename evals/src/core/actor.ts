import type { Call } from "./calls.js";

/** The part of a scripted second actor that the driver serializes. */
export interface Tickable<A> {
  readonly actions: readonly A[];
  tick(now: number, trigger: Call | undefined, source: "timer" | "post-tool"): Promise<void>;
}

/** Serializes timer and post-tool ticks, including their evidence writes, and drains on shutdown. */
export class ActorDriver<A> {
  private queue: Promise<void> = Promise.resolve();
  private active = true;
  error?: Error;

  constructor(
    private readonly actor: Tickable<A>,
    private readonly changed: (actions: A[]) => Promise<void>,
    private readonly now: () => number = Date.now,
  ) {}

  tick(trigger?: Call, source: "timer" | "post-tool" = "timer"): Promise<void> {
    if (!this.active) return Promise.resolve();
    const next = this.queue.then(async () => {
      if (this.error) throw this.error;
      const before = this.actor.actions.length;
      await this.actor.tick(this.now(), trigger, source);
      if (this.actor.actions.length > before) await this.changed(this.actor.actions.slice(before));
    });
    this.queue = next.catch(error => { this.error = error instanceof Error ? error : new Error(String(error)); });
    return next;
  }

  async close(): Promise<void> {
    this.active = false;
    await this.queue;
    if (this.error) throw this.error;
  }
}
