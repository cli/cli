# Tentative eval design notes

These notes capture practices that worked for the event-driven `gh` evals. They
are hypotheses to retain and test, not a universal framework.

## Scenario ownership

A typed scenario definition can own:

- fixture shape and initial state;
- ordered second-actor steps;
- independently observable task checks.

Shared modules can own cross-scenario timing, command observation, cleanup, and
grading policy. This kept scenario behavior readable without central
scenario-name conditionals.

## Actor timing

Use a separately authenticated actor. Trigger it from observable readiness or
agent progress rather than a fixed control delay.

- A live-only stream needs an explicit attached/ready signal.
- To test the startup gap, deliver a change before attachment is guaranteed and
  require a post-listener snapshot to recover it.
- Serialize timer and post-tool ticks so one change cannot be posted twice.
- Verify pushed content before posting a follow-up review or approval.
- Approve the exact current head, not merely the PR.

An actor fallback can prevent an indefinitely blocked trial, but record it
explicitly. A fallback-triggered change should not count as evidence that the
agent established the required wait.

## Evidence and replay

Persist a deeply validated, versioned record containing:

- scenario and stimulus variant;
- enabled interventions and skill state;
- actual command argv;
- timestamped stdout and stderr;
- command exit status;
- actor actions and reviewed heads;
- final GitHub and local snapshot.

Offline replay should re-run deterministic task, route, and polling graders
without credentials, network, or a model. A recorded semantic judgment may be
preserved as provenance, but replaying it is not the same as re-grading.

Match exact evidence. A running wait proves nothing unless its output identifies
the scripted event, or a snapshot taken after listener attachment recovers the
exact change. Thin events may require one event-triggered follow-up read.

## Polling

Define polling mechanically instead of asking a model to infer intent. A useful
first definition is repeated successful reads of the same status projection
within one external-change epoch.

This permits:

- one initial inspection;
- one read after each actor change;
- reads needed to resolve a thin event.

It rejects repeated unchanged reads used as a waiting mechanism. Keep route and
polling graders independent: observing one event does not excuse later polling,
and avoiding polling does not prove that the event route worked.

## Prompts and treatments

The normal prompt should sound like a user request. Do not prescribe the desired
command or say "do not poll."

Use separate variants for:

- design probes, where the agent is told a capability exists without syntax;
- product interventions, such as an opt-in command hint;
- negative controls, where a required event or recovery step is absent.

For skill comparison, keep the baseline skill unchanged and make treatment an
auditable baseline-plus-delta. Isolate the evaluated environment so the skill is
the only intended off/on difference.

## Credentials and cleanup

Keep identities distinct:

- preview-enabled subscriber/agent user;
- separate reviewer/merger actor;
- optional fixture-management identity;
- model-access token.

Do not infer authorization from ambient credentials. In particular, do not
default the evaluated GitHub identity to `gh auth token`.

Journal resources before the agent begins. Cleanup should cover subscriptions
and repositories, retain the original execution error, and separately report
cleanup failure. Event subscriptions must be removed by the same owning user.

## Feedback loops

Use progressively more expensive checks:

1. Unit-test command and actor behavior.
2. Replay captured or synthetic evidence offline.
3. Run a no-model live smoke when the real API protocol matters.
4. Run one shortened model trial.
5. Use the checked-in budget for final off/on measurement.

The model trial should measure adoption, not debug command semantics, actor
timing, cleanup, or deterministic grader logic.

## Failure patterns observed so far

| Failure | Lesson |
| --- | --- |
| Bespoke TypeScript runner duplicated Vally | Keep the wrapper thin and use native plugins |
| LLM-generated routing facts varied | Derive facts from command evidence |
| Stream and snapshot launched in parallel | Require explicit readiness before snapshot |
| Numeric and named API URLs differed | Match both canonical forms |
| Approval arrived during work | Tick the actor at post-tool boundaries |
| Cleanup 503 suppressed a satisfied wait result | Emit result evidence while retaining cleanup failure |
| Model loop took minutes per grader change | Add replay and no-model smoke seams |
| Hint targeted only `pr view` | Record that agents may poll through other commands |

## Open questions

- Which polling definition generalizes beyond PR status without false positives?
- How many repetitions are needed before treating a skill effect as stable?
- Which command hints improve discovery without becoming noisy script output?
- When should semantic review become advisory rather than scored?
- How should recorded semantic judgments participate in offline replay?
- Would a comparison canvas make skill-off/on timelines, commands, actor
  changes, grader outcomes, polling, and cleanup easier to inspect side by side?
