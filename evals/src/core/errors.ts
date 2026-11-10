/** Errors that describe the harness or shared account, never retried by Vally. */
export class InfrastructureError extends Error {
  readonly retryable = false;
}
