import { ApiError, GitHub } from "../../core/github.js";
import { record } from "../../core/io.js";

export interface Subscription {
  id: string;
  resource_url: string;
  event_types: string[];
}

export const eventTypes = [
  "com.github.pull_request.state.changed.v1",
  "com.github.pull_request.review.submitted.v1",
  "com.github.pull_request.head.changed.v1",
  "com.github.pull_request.merge_conflict.v1",
] as const;
export type EventType = typeof eventTypes[number];

/** The GitHub client with the Event Subscriptions API and the event fixtures' file. */
export class EventsGitHub extends GitHub {
  // The subscription API returns 503 (with Retry-After) or 504 for transient store and access-check failures, and
  // documents both as retryable. Its operations are idempotent, so retrying is safe. The preview route occasionally
  // answers a valid listing with a generic 404. A listing can never be legitimately missing, so treat that as
  // transient; a 404 for a single subscription still means it is gone.
  protected override transient(method: string, path: string, status: number): boolean {
    if (!path.startsWith("/event-subscriptions")) return false;
    const listing = method === "GET" && /^\/event-subscriptions(\?|$)/.test(path);
    return status === 503 || status === 504 || (listing && status === 404);
  }

  /**
   * Returns the caller's subscription for exactly this PR and event-type set, creating it if absent.
   * Returns undefined when the set does not exist and the caller is at the subscription limit:
   * creating an existing subscription succeeds even at the limit, so nothing exists to clean up.
   */
  async subscribe(prUrl: string, types: readonly EventType[]): Promise<Subscription | undefined> {
    let body: Record<string, unknown>;
    try {
      body = record(await this.api<unknown>("POST", "/event-subscriptions", {
        resource_url: prUrl, event_types: types,
      }));
    } catch (error) {
      if (error instanceof ApiError && error.status === 422 && /Subscription limit exceeded/.test(error.message)) {
        return undefined;
      }
      throw error;
    }
    if (typeof body.id !== "string" || !/^sub_[A-Za-z0-9]+$/.test(body.id) || body.resource_url !== prUrl) {
      throw new Error("Subscription response is missing the ID or matching resource URL needed for safe cleanup");
    }
    return { id: body.id, resource_url: body.resource_url, event_types: [...types] };
  }

  /** Lists every active subscription of the caller, following pagination links. */
  async subscriptions(): Promise<Subscription[]> {
    const all: Subscription[] = [];
    let path: string | undefined = "/event-subscriptions?per_page=100";
    for (let pages = 0; path; pages++) {
      // The API lists at most twice an owner's limit, so this many pages means the links are looping.
      if (pages >= 10) throw new Error("Event subscription listing did not end");
      if (!/^\/event-subscriptions\?/.test(path)) throw new Error("Unexpected event subscription pagination link");
      const page: { body: unknown; next?: string } = await this.page<unknown>("GET", path);
      const body = record(page.body);
      if (!Array.isArray(body.subscriptions) || body.subscriptions.length > 100) {
        throw new Error("Invalid event subscription listing");
      }
      all.push(...body.subscriptions.map(value => {
        const subscription = record(value);
        if (typeof subscription.id !== "string" || !/^sub_[A-Za-z0-9]+$/.test(subscription.id)
          || typeof subscription.resource_url !== "string"
          || !Array.isArray(subscription.event_types)
          || !subscription.event_types.every(type => typeof type === "string")) {
          throw new Error("Invalid event subscription listing");
        }
        return {
          id: subscription.id,
          resource_url: subscription.resource_url,
          event_types: subscription.event_types,
        };
      }));
      path = page.next;
    }
    return all;
  }

  greeting(repo: string, ref: string): Promise<string> {
    return this.fileContent(repo, "greeting.txt", ref);
  }
}
