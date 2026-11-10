# Eval execution modes

If the caller has not explicitly said who should execute, use the question tool:

```text
Who should run this eval?
```

Offer:

1. The agent runs it and reports the result.
2. The caller runs it in their terminal and pastes the result for interpretation.

## Agent-run mode

Follow the credential setup, authorization, cheapest-first gates, live run,
cleanup verification, and artifact reporting. The caller's request to run an
eval does not authorize an arbitrary organization or credential source.

## Caller-terminal mode

Do not access credentials or create resources. Provide:

- the exact command;
- the environment variables the caller must populate for each credential role;
- expected resource effects and cleanup behavior;
- the result sections or artifact paths to return.

Then stop until the caller supplies output. Interpret their output using the
same task, route, polling, semantic, infrastructure, and cleanup distinctions.

If the caller already said “you run it” or “I’ll run it,” honor that choice
without asking again.
