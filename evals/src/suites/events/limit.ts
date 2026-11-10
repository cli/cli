/**
 * Detects the subscription API refusing a new subscription because the user already holds the
 * maximum. That is a property of the shared account, not the agent, so the trial is not gradable.
 */
export function subscriptionLimitHit(events: unknown[]): boolean {
  return events.some(value => {
    if (typeof value !== "object" || value === null) return false;
    const type = (value as Record<string, unknown>).type;
    return type === "tool.execution_complete" && JSON.stringify(value).includes("Subscription limit exceeded");
  });
}
