# Declarative controllers

## Contents

- [Rule shape](#rule-shape)
- [Pause and resume](#pause-and-resume)
- [Fidelity and evidence](#fidelity-and-evidence)

Use the [shared runtime interface](runtime-interface.md) to launch, observe,
and close a run. Load this reference only when authoring or running a controller.
The calling AI chooses the strategy; the Go owner executes deterministic rules
and does not invoke another model. The Node adapter has no controller logic.

## Rule shape

A controller contains bounded literal rules, not executable code.
Use the actual observation revision instead of the illustrative value below:

```json
{
  "op": "run_controller",
  "revision": 3,
  "reason": "Follow the supplied interactions where their screens are recognized.",
  "controller": {
    "id": "known-menu",
    "maxActions": 3,
    "maxDurationSeconds": 15,
    "waitForMatchMs": 1000,
    "rules": [
      {
        "id": "continue",
        "when": {"screenContains":"Choose","selected":"Continue"},
        "action": {"type":"key","key":"enter"},
        "reason": "The case specifies Enter on the selected Continue option.",
        "maxMatches": 1
      }
    ]
  }
}
```

The first eligible matching rule runs. Conditions combine literal
`screenContains`, `questionPrefix`, and `selected` with AND. Regular expressions,
generated predicates, and arbitrary code are not accepted.

`maxMatches` defaults to one per rule
and source hash for the lifetime of the run. Explicit repetitions remain
bounded by controller and run limits. Reusing the same controller resumes its
remaining rules; it does not reset match counts. Controller bounds cannot relax
the enclosing run's limits.
The duration budget covers actions, typing delays, menu navigation, and waiting
for acknowledgements, not just the gaps between rules.

## Pause and resume

Terminal input delivery can finish before its echo or next prompt arrives.
`waitForMatchMs` bounds how long the controller observes for a matching rule
before declaring the current screen uncovered. It defaults to 1000 milliseconds
and accepts 0 through 60000, within the controller's duration limit. It sends no
input, resets no match counts, and cannot waive an exact action's timing window.

No matching rule after that wait produces `controllerStatus: "paused"` with
`reason: "unhandled_state"`. The terminal remains available to the live agent
until it exits or the run reaches its limits. Observe the new state, then
continue with an authorized live action or resume an appropriate controller.
Do not turn an unexpected prompt into unprovided input in an exact case.

## Fidelity and evidence

Each rule uses the same [canonical actions and timing](run-contract.md#actions-and-constraints)
as live input. In exact mode, its action must match the next supplied step,
including timing fields. Controller execution cannot replace a specified key
sequence with a semantic selector or add unprovided interactions.

An exact action that fails after delivering partial input stops the run as
blocked. Do not replay its already delivered prefix. This differs from a
controller pausing after a completed action.
If the controller deadline expires during input, the request reports
`controller_limit` and warns that delivery may be partial. Expiry before any
input pauses the controller without executing the pending action.

Controller sources are saved as JSON under `capture/controllers/<sha256>.json`.
Events retain the source hash, matched observations, input reasons, execution
mode (`ai`, `controller`, or `host`), and guard evidence. Controller strategy
does not change the contract's `exact` or `explore` fidelity.

Source documents must not contain actual credentials, including secrets split
across literal steps. See [privacy and environment](runtime-interface.md#environment-and-privacy)
for the shared safeguards and limitations, and [maintenance](maintenance.md)
for isolated validation commands.
