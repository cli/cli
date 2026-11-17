# Eval credential setup

Never ask the caller to paste a token into chat. Never retrieve, mint, or search for credentials yourself.

## Required roles

| Variable | Identity and purpose |
| --- | --- |
| `GH_EVAL_TOKEN` | Preview-enabled user used by evaluated `gh`, Git pushes, and event subscriptions |
| `GH_EVAL_ACTOR_TOKEN` | Different reviewer/merger identity, usually a short-lived App installation token. Required by the events suite and other suites with a second actor |
| `GH_EVAL_FIXTURE_TOKEN` | Optional repository creator/deleter; defaults to `GH_EVAL_TOKEN` |
| `COPILOT_GITHUB_TOKEN` | Model access only |

For the current classic PAT setup, `GH_EVAL_TOKEN` needs `repo` and `read:org`.
It also needs `delete_repo` when no separate fixture credential deletes the
sandbox repository.

## Environment variables

The evals read every credential from environment variables. Validate presence without printing values:

```sh
test -n "$GH_EVAL_TOKEN"
test -n "$GH_EVAL_ACTOR_TOKEN"  # events and other suites with a second actor
test -n "$COPILOT_GITHUB_TOKEN"
```

Do not assume exports made in a separate interactive terminal are visible to the agent's tool process. Reject empty values, embedded whitespace, and an actor token equal to the subscriber token.

When any role is missing, stop and use the question tool to ask the caller to populate the missing variables in the environment of the process that runs the eval, or to switch to caller-terminal mode. Do not look for tokens anywhere else.

## Validation

Before creating resources:

- confirm the host and sandbox organization;
- verify the subscriber token's scopes without printing it;
- verify subscriber and actor identities differ;
- state which identity creates and deletes fixtures.

Unset transient variables after the run.
