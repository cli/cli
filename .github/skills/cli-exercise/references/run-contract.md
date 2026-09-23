# Run contract

## Contents

- [Contract shape](#contract-shape)
- [Fidelity](#fidelity)
- [Environment and authorization](#environment-and-authorization)
- [Actions and constraints](#actions-and-constraints)
- [Expected outcomes](#expected-outcomes)
- [Output and provenance](#output-and-provenance)

## Contract shape

The entry skill preserves the original request and writes one immutable contract
per executable invocation. A case represents one behavior and has at most one
exercise invocation; several commands may prepare or validate that behavior.
Keep their case ID and actual execution order when aggregating results. Finish
the case's preparation, exercise, and validation before the next case; approved
explicit cleanup can follow validation. See [recording sessions](recording-sessions.md).
Do not silently split or reorder an exact caller case to fit this structure;
ask about conflicts, and honor explicit narrower coverage.

Do not invent shell state between separate processes. If a case requires one
continuous shell or program, identify that executable and express the supplied
interactions in that session.

Contracts and recordings live in a private temporary workspace outside the
repository. Never place credentials in a contract, including `source.text`.
Retain non-secret source instructions and explicit approved secret references.
If literal fidelity would require disclosing a real secret, report blocked.

```json
{
  "schemaVersion": 1,
  "caseId": "example",
  "mode": "exact",
  "source": {
    "kind": "explicit",
    "text": "The caller's original case, preserved verbatim."
  },
  "goal": "Observe the requested behavior without changing the supplied case.",
  "workspace": "/absolute/private/workspace",
  "command": {
    "executable": "/absolute/path/to/program",
    "args": ["--help"],
    "sha256": "<64 lowercase hexadecimal characters>",
    "version": "<observed version>",
    "cwd": "work"
  },
  "environment": {
    "values": {},
    "pass": []
  },
  "authorization": {
    "status": "approved",
    "basis": "The explicit caller request or a recorded, specific approval.",
    "effects": []
  },
  "terminal": {
    "columns": 100,
    "rows": 28,
    "fontPath": "/absolute/path/to/monospace-font.ttf",
    "fontSize": 20,
    "fps": 30,
    "background": "#0d1117",
    "foreground": "#e6edf3"
  },
  "limits": {
    "maxActions": 100,
    "maxDurationSeconds": 600,
    "idleTimeoutSeconds": 120
  },
  "steps": [],
  "constraints": {
    "deny": []
  },
  "expectations": [
    {
      "id": "exit",
      "type": "exit_code",
      "value": 0
    }
  ],
  "output": {
    "formats": ["gif"],
    "timing": "condensed"
  }
}
```

## Fidelity

- `exact` preserves supplied commands, argument values, interaction order,
  timing requirements, and assertions. `steps` is the ordered canonical action
  list. A different input action must be rejected before execution.
- `explore` permits the agent to choose interactions within the approved goal
  and constraints. It does not permit new executables, targets, credentials, or
  effects outside the contract.
- The runtime starts the exact `command` once. Starting a different command
  requires a different reviewed contract, not typing an unapproved shell command.
- Empty `steps` in exact mode means no interactive input is authorized. Pure
  observations do not add test steps. Mechanical waits are permitted only when
  they do not change a supplied timing requirement.
- If the caller specified literal keypresses, do not replace them with a
  semantic `select` operation. If the caller specified an option by name, a
  mechanical selector may locate that option in the current menu.
- A controller is an execution strategy, not authority to change exact steps.
  Live AI and controller actions pass through the same fidelity checks.

## Environment and authorization

`command.cwd` is relative to the workspace. The runtime creates isolated home,
configuration, cache, and temporary directories there. It must not reuse the
operator's profile or silently run in the repository checkout.

For related commands in a [recording session](recording-sessions.md), optional
`stateDirectory` names a separate private directory shared by those invocations.
Then `command.cwd` and the isolated profile directories are rooted there, while
each invocation keeps its own capture `workspace`. The two directories must not
overlap. Omitting this field preserves per-run isolation.

`environment.values` contains reviewed non-secret values. Each `environment.pass`
entry is `{"name":"VARIABLE","secret":true}` or the same with `secret:false`.
Only explicitly approved process variables may pass through; secret values are
read at launch and never serialized into the contract or evidence. Missing
requested variables are errors.

The authorization record documents the caller's decision. It is not a
cryptographic authorization mechanism or an OS sandbox. Skill instructions must
obtain actual user approval when the request does not already authorize an
effect. Runtime allowlists do not make arbitrary programs safe.

## Actions and constraints

Canonical effectful actions use `type`: `text`, `key`, `select`, `toggle`,
`resize`, or `click`. Their fields are respectively:

```json
{"type":"text","text":"Mona"}
{"type":"key","key":"enter"}
{"type":"select","label":"Continue"}
{"type":"toggle","label":"Verbose","checked":true}
{"type":"resize","columns":80,"rows":24}
{"type":"click","x":10,"y":5}
```

Optional `timing` belongs to the canonical action, not the surrounding request:

```json
{
  "type": "text",
  "text": "Mona",
  "timing": {"delayBeforeMs":1000,"maxDelayBeforeMs":1500,"typingDelayMs":35}
}
```

The lower and upper delay are measured from completion of the preceding action,
or from launch for the first action. `typingDelayMs` applies only to text and
sets the minimum interval between successive characters. Observations and notes
do not reset those clocks. Waits can satisfy a lower bound but cannot waive an
upper bound. Preserve supplied timing fields in exact actions and controllers.

The available action set is deliberately bounded. Unsupported actions must be
reported, never silently approximated. Input after the target exits is forbidden.
Observation, waiting, and annotation operations never authorize extra input.

A deny entry matches action fields, optionally under a live-screen condition:

```json
{
  "action": {"type":"key","key":"enter"},
  "when": {"questionPrefix":"? What's next?", "selected":"Submit"},
  "reason": "Publishing was not authorized for this case."
}
```

Omitting `when` makes the denial unconditional. Conditions can use literal
`screenContains`, `questionPrefix`, and `selected` values. These are explicit
case constraints, not a built-in rule that every command must cancel.

## Expected outcomes

Built-in terminal assertions are `exit_code`, `screen_contains`, and
`screen_not_contains`, each with an `id` and `value`. Screen assertions describe
the final visible terminal, not separate stdout/stderr streams. Do not use an
echoed input as evidence of product output.

Other supplied expectations use `{"id":"...","type":"manual","description":"..."}`.
The evidence skill must verify them with approved independent observations and
cite that evidence, or mark them blocked. The runtime must not silently pass
unsupported assertions.

No assertions means `observed`, not `passed`. Keep case outcome, capture status,
and rendering status separate. A renderer failure is not a product regression.
Pass/fail is against expectations, not a preference for exit zero: an expected
nonzero exit may pass, while exit zero does not excuse a contradicted assertion.

## Output and provenance

GIF and MP4 are supported output requests. `output.captions` defaults to true;
set it to false when the caller requires an unannotated recording. Optional
`terminal.fontFallbacks` lists explicit additional font paths for missing glyphs.
These fonts are the rendering defaults. The evidence command can select other
fonts for a new rendering without editing this contract; see
[render-time font selection](runtime-interface.md#offline-evidence-rendering).
`terminal.fps` must be a whole number from 1 through 60.
`output.timing` defaults to `condensed` when omitted. Use `realtime` when the
caller requests full timing; an existing contract's explicit timing is preserved.
Condensed presentation retains original capture timings and records timing
adjustments in rendering metadata, without repeated on-screen timing labels.
It requires no separate approval and cannot stand in for a timing assertion.
Unannotated output (`captions:false`) must explicitly select `realtime` to
preserve the raw recording's timing as well as its appearance.

Normal final deliverables use a session manifest even for one case. An annotated
session adds a task-specific overview, derived fidelity label, and case results
with 10 seconds per readable page, plus continuous whole-video progress. The
existing manifest fields provide these inputs; do not add summary fields to
this invocation contract. Explicit unannotated real-time output has no added
overview, invocation header, phase labels, explanation panel, or progress line.

Users select these preferences in their request, for example "a condensed MP4"
or "both a real-time recording and a short preview." Normalize the initial
preference into `output`. Additional cuts can be rendered afterward using the
evidence command's presentation flags without altering this contract.

Record the command identity, contract hash, all input actions, observed terminal
states, actual exit status, controller sources, and inspection coverage.
Annotations describe the run but must never overwrite the product's output.
The displayed invocation is derived from the immutable recorded executable and
arguments; it is not permission to type or execute another command for filming.

The contract hash is SHA-256 of the exact UTF-8 `contract.json` bytes. Preserve
those bytes in the workspace. `result.json` also records `stepsCompleted` and
`expectedSteps` so incomplete exact cases cannot be promoted by later reporting.
Retain runtime `startedAt` and `finishedAt` metadata for session chronology.
Do not invent timestamps for legacy captures; render them only with an explicit
`execution order unverified` warning.
