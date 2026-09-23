# Input examples

These examples describe routing, not output from a real product. Resolve
placeholders and authorization before execution.

## Explicit case

Input:

> Record this case exactly using the identified local fixture executable:
> enter `Mona`, press Enter, and expect `Hello, Mona` with exit code 0.
> Return a GIF. Do not add other checks.

Route: `exact`.

Preserve the text, key, expected result, and GIF request. Check prerequisites,
then execute the canonical steps through the exact-action gate. Do not add an
empty-input test, try another name, or load the goal-exploration flow.
Use one case and a one-case session manifest for the final GIF. The session
overview does not authorize extra setup, validation commands, or typed input.
Choose a brief behavior title such as `Name input greets Mona`.

## Broad goal

Input:

> Show how to upload an attachment.

Route: `explore`, after resolving missing context.

Determine the selected executable, file, destination, and permitted effects.
Ask for missing authority rather than using the current repository or account
by assumption. Once authorized, choose useful checks and whether to use a
controller or live inspection. Record which expectations were supplied or inferred.

## Mixed request

Input:

> First record these two supplied cases exactly. Then explore whether filtering
> changes preserve an existing selection.

Route each case separately. Keep the supplied cases and ordering locked. Give
only the final exploration case freedom to choose its path. Do not apply the
broader goal to the earlier explicit cases.
The session's fidelity label is `Mixed run`, derived from its exercise
contracts, even if every preparation and validation helper uses exact steps.

## One behavior per case

Input:

> Exercise saving a prompt setting, storing an alias, and expanding an alias
> using the selected gh binary and isolated local configuration. Show the video.

Plan three behavior cases, not one broad "configuration and aliases" case:

1. `gh config set disables prompts`: establish enabled prompts, exercise the
   change once, then immediately validate the disabled value.
2. `gh alias set stores an alias`: establish that the example alias is absent,
   exercise storing it once, then immediately validate its saved definition.
3. `gh alias expands to a command`: prepare the alias, exercise its expansion
   once, then immediately validate the result.

Record each case's preparation, exercise, and validation before beginning the
next; approved explicit cleanup follows validation. Run titles explain purposes,
such as `Establish enabled prompts` and `Confirm prompts are disabled`.
Multiple preparation or validation commands are fine when they serve that one
behavior. Do not collect all preparation first or all validation at the end.

Use `evidence --session` to return one media-only video with the task-specific
overview and whole-video progress. Do not deliver unrelated per-invocation clips
or rearrange a batched capture into an apparently sequential one.

If a caller instead supplies an exact multi-behavior case or requires a different
phase order, ask about the conflict. Do not silently split, reorder, relabel
exercise invocations as preparation, or add input to make it fit.

## Dependency approval declined

Input:

> Record this case. If Tuistory is missing, do not install anything.

Run checks only. If the required capability is missing, report blocked with the
specific missing component and remedy. Do not invoke an installer, switch to
another recorder, or claim the case passed.

## Output preferences

Users can state output preferences directly:

> Record this as a real-time GIF.

> Give me a condensed MP4 that shortens idle pauses.

> Keep the full recording and also give me a quick preview.

Format and timing are independent. Use GIF for an unspecified terminal
animation, MP4 for a video request, and condensed timing by default.
An explicit real-time request keeps full timing. When both variants are wanted,
render both from the same capture rather than repeating the command.

> Return a real-time recording with no annotations.

Keep that cut unannotated: no overview, phase labels, invocation header, lower
explanation panel, progress line, condensed pauses, or added reading holds.

> Make a video exercising this PR's changes.

Return the MP4 without generating HTML. Preserve machine-readable evidence
internally.

> Give me an interactive report where I can jump between cases.

Add `--html` and return the report with the video. Do not ask a redundant output
question when the caller's wording already resolves the choice.

## Long summaries and honest outcomes

Input:

> Record all of these behavior cases, including expected failures, and give me
> one video.

Use the task-specific manifest title, the fidelity derived from exercise
contracts, and a brief title/result row for every case. Add readable summary
pages as needed and show each for 10 seconds; do not limit the entire intro to
10 seconds or omit cases. Progress continues across these pages, holds, and
chapter boundaries.

An expected invalid-input error with the specified nonzero exit can pass.
An exit-zero run with a contradicted assertion fails. Validation is green only
for passed expectations, red for failed expectations, and explicitly non-passing
for blocked, observed, or unknown results. Always include text labels.

Show the recorded executable and arguments at the top of the terminal area,
followed by the real output. Explain the behavior below, rather than repeating
`Command: ...`. Do not type or run another shell command to create this display.

If legacy captures lack runtime timestamps, retain the explicit
`execution order unverified` warning. Video chapter order is not proof of
historical execution order.
