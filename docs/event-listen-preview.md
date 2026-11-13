# Event listening preview

`gh event listen` and `gh event subscription` are in preview. This describes their current behavior, which may change.

## `gh event listen`

```sh
gh event listen --resource https://github.com/OWNER/REPO/pull/123 --event com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1
gh event listen --resource https://github.com/OWNER/REPO/pull/123 --event com.github.pull_request.review.submitted.v1 --timeout 30m
gh event listen --resource https://github.com/OWNER/REPO/pull/123 --event com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1 --follow
gh event listen --subscription SUBSCRIPTION_ID
```

`gh event listen` is a single, general purpose command for waiting on GitHub events. It creates a subscription for the resource and event types, or reuses the existing one for the same set, attaches to its event stream, and prints each event on stdout as one compact CloudEvent JSON object per line. Keep-alives are not printed.

`--event` takes a full event type name, such as `com.github.pull_request.review.submitted.v1`, and can be repeated or list several types separated by commas; there are no short aliases. gh does not validate them: the API rejects unsupported types with `422 Unsupported event types: ...`. `--resource` is not validated or rewritten either: the API rejects anything but a pull request URL, including a URL with a suffix such as `/files`. Its host only selects where the request is sent; without one, the default host is used.

`--subscription` attaches to an existing subscription by ID instead of creating one. It cannot be combined with `--resource` or `--event`.

## Output and readiness

Events are thin: `type` says what changed and `data.api_url` points at the changed object, such as the review, the pull request, or the check suite. Consumers fetch it when they need details. Check suite events also carry `data.conclusion`; a pull request usually has several check suites, each sending its own event, so consumers waiting for all checks read them after each event.

`--jq` and `--template` format each event separately, like `gh api`. They are mutually exclusive and validated before subscribing. jq results are printed one per line, with strings unquoted. An expression or template that fails on an event ends the command rather than reconnecting. Output is JSON whether or not stdout is a terminal.

`Listening for events.` is written to stderr each time the stream is attached, including after reconnecting (`Listening for events on a new subscription.` when the subscription was created again), followed by a reminder that earlier changes are not reported. The API is live-only, so callers that need the current state start the listener, wait for this line, and then take their snapshot. Starting the listener and the snapshot in one parallel batch can miss an event.

## Lifecycle

By default, the command exits with status 0 after the first event, and then notes on stderr that it can be run again to wait for the next event. `--count N` exits with status 0 after N events. `--follow` listens until the command is interrupted, and cannot be combined with `--count`. `--timeout` exits with status 8 when it is reached first. Exiting on the first event by default suits agent harnesses, which resume an agent when a background command exits, not when it prints.

Network errors, `5xx` responses, streams closed without an end frame, and `reconnect` end frames trigger a reconnect with jittered exponential backoff, capped at 30 seconds. The backoff resets only after a stream delivers a heartbeat or an event, not when it merely attaches, so a server that closes streams right away is not reconnected to every second. A `Retry-After` header on a `503` raises the wait to at least that many seconds. Creating the subscription retries the same errors. A stream that delivers no data, not even a heartbeat, for 45 seconds is treated as dead and reconnected; the server sends a heartbeat every 9 seconds.

When a subscription expires, or the stream reports it missing, the command creates it again if it was started with `--resource`; a second consecutive miss is final. Started with `--subscription`, a missing or expired subscription is final. A deleted subscription is final in both modes, so deleting a subscription stops its listeners. Other HTTP errors, including `401` and `403`, and other `subscription_ended` outcomes are final. A `401` gets the usual `gh auth login` hint.

Subscriptions are not deleted on exit. They are idempotent per user, resource, and exact event set, so independent listeners share them safely, and they expire after 24 hours. Each account can hold a limited number of active subscriptions; a create beyond the limit returns `422 Subscription limit exceeded`, which the command reports as a rejected request that may have reached the limit. Other `422` responses list the invalid fields, such as `resource_url`, and are reported as an invalid subscription request. A gateway routing failure returns the same `404 Not Found` body as a missing or inaccessible pull request, so the two cannot be told apart.

Preview requests use the configured HTTP client. Only github.com is supported; GitHub Enterprise Server and GHE.com hosts fail before any request with "event subscriptions are only available on github.com". The credential's account must be enabled for the preview: the user for a user token, or the user or organization the app is installed on for a GitHub App installation token. Enabling an organization admits installation tokens for the apps installed on it, but not its members' user tokens.

## Discovery hints

When `internal/agents` detects an agent caller, a successful `gh pr create` of a github.com pull request, a successful non-browser `gh pr view` of a github.com pull request that is not merged or closed, and a successful `gh api` GET of a github.com `repos/OWNER/REPO/pulls/NUMBER` endpoint or any of its subpaths, given as a path or as an absolute `https://api.github.com` URL, print a hint on stderr for waiting on its events instead of polling, addressed to agents that need to wait for the pull request to be reviewed, merged, or closed before continuing. The hint puts a copyable `gh event listen` command on its own line, with full event type names. It notes that the command prints the first event as a JSON line and exits, and that it can be run again to wait for the next one. It deliberately does not mention `--follow`: agents offered it chose it even for one-off waits, ran it in the background, and were never resumed. It suggests `--timeout` with an example. It also notes that events carry no details, that changes made before the listener starts are missed so the pull request should be read once it is running, and points to `gh event listen --help` for other event types. `gh api` does not know the pull request state, so its hint is also shown for merged or closed pull requests. Human invocations, other hosts, stdout, JSON output, and exit status are unchanged. The `gh pr` and `gh pr view` help texts also point to `gh event listen` for github.com, and, like the `gh event` help, state that changes made before the listener starts are not reported.

For an agent caller, `gh run watch` and `gh run view` of a workflow run that is not completed and belongs to a github.com pull request print a similar hint on stderr, addressed to agents that need to wait for the checks of the pull request to finish. It suggests listening to `com.github.pull_request.check_suite.status.changed.v1` for the pull request, with the same notes, except that it explains that each check suite sends its own event with its conclusion, and that the checks should be read with `gh pr checks` after each event, running the listener again while any are pending. `gh run watch` prints it before it starts watching, so it is seen even if the watch is stopped. `gh run view` prints it with `--json` too, but not with `--web` or `--log`. The `gh run watch` and `gh run view` help texts point to `gh event listen` for waiting on the workflow runs of a github.com pull request.

## Eval use

The shipped `gh` skill at `skills/gh` documents `gh event listen`. Evaluate it from the working tree, or compare it with the version on trunk, which has no event guidance until this change merges:

```sh
# The skill on this branch
evals/run --live --gh ./bin/gh --gh-skill ./skills/gh --model MODEL_ID --skill on notify-review

# The skill on trunk
evals/run --live --gh ./bin/gh --gh-skill-ref trunk:skills/gh --model MODEL_ID --skill on notify-review
```

The route grader accepts `gh event listen` output for each scripted change, including from a listener stopped after it delivered the event. Output reshaped with `--jq` or `--template` cannot be matched to a specific event, so it counts when the listener was bound with `--resource` to the fixture pull request and an `--event` set that covers the change. Direct `gh api` streams remain a separate, non-passing route under the current eval contract.
