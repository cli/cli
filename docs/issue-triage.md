# Issue triage reports

[Development guides](README.md)

The [issue-triage workflow](../.github/workflows/issue-triage.md) runs `gh` against
test data. It suggests labels for maintainer approval, posts one short comment,
and attaches separate Markdown reports when applicable:

- **Duplicate analysis:** search evidence and limitations.
- **Label reasoning:** generated from the same reasons and confidence sent to the
  label suggestion system, not a second explanation written by the agent.
- **Reproduction log:** test conditions, actual `gh` commands, output, exit
  statuses, tested versions, and coverage limits.
- **Investigation notes:** source evidence, likely causes, alternatives, and
  remaining questions.

The visible comment is limited to 150 words, including attribution and the
collapsed **Supporting reports** summary. It shows a small command/output example,
not the test backend's setup. Test data must not be presented as live GitHub data.
The raw evidence remains a workflow download.

A confirmed duplicate takes an early exit: suggest only `duplicate`, publish the
one-sentence explanation, and stop. No source investigation, reproduction, or
automatic issue closure follows that decision.

## Feature requests

After the duplicate check, verify what the current released `gh` already supports
using command help, documentation, or released source. Distinguish full support
from partial support, a workaround, or unreleased behavior. A historical issue's
claim that a feature is missing is not proof that it is still missing.

If the behavior is supported, give a short verified answer rather than suggesting
an enhancement for it. If a gap remains, check that gap for duplicates and suggest
only genuinely new work. Ask for clarification only when it would help identify
the gap. Do not promise implementation.

Keep the checked version or ref and supporting evidence in **Investigation notes**.
If the remaining gap is a confirmed duplicate, put the evidence in **Duplicate
analysis** and take the normal early exit. This availability check does not require
a bug reproduction, bug priority, or feasibility study. Do not claim released
support based only on trunk source.

## Model selection

Leave `engine.model` unset so the workflow uses `gh aw`'s normal model selection.
`GH_AW_MODEL_AGENT_COPILOT` selects the triage model, and
`GH_AW_MODEL_DETECTION_COPILOT` selects the security-check model.
Each falls back to `GH_AW_DEFAULT_MODEL_COPILOT`, then its `gh aw` default.

## Prepared builds

Trusted preparation records each executable's path and observed `gh version`
line in `/opt/ghmock/builds.txt`. This includes the selected source build, the
latest release alias, and any prepared reported releases. Failed version checks
produce a warning and are omitted; no available builds produces an explicit
empty inventory. Preparation remains nonfatal.

The agent reads this inventory instead of discovering versions by running the
binaries directly. Like the prepared executables, the inventory is mounted
read-only in the sandbox. The existing shell permissions are unchanged.

## Publishing

The agent calls `publish_triage` once after its label suggestions. The separate
publishing job runs only after the security check succeeds. It checks out trusted
source at the workflow's commit, not files modified by the agent.

[The publisher](../.github/scripts/issue-triage-report.cjs) validates the output
and uses the pinned gh-aw sanitizer. It creates Markdown issue artifacts through
`gh api`, which returns their numbers and canonical URLs. These are issue
documents, not the archives managed by `gh run download`. The publishing job uses
a checksum-pinned `gh` release and a repository-scoped GitHub App token with
`issues: write`. The agent does not receive that token.

Synthetic repositories use the owner `example.invalid`, including URLs returned
by reproduction fixtures. GitHub account names cannot contain periods, so the
publisher skips live visibility lookups only for that exact owner. Captured
commands and output remain unchanged. Other GitHub references, including those
inside command output, still block public publication when they are private or
their visibility cannot be verified.

Artifact numbers are tracked per report type in checksum-protected bot comment
metadata. The latest verified comment identifies the current report of each type.
Ownership checks also use the bot identity, content checksum, and
version history. Readable human-owned, renamed, edited, or ambiguous artifacts
are left unchanged. A saved report absent from a successful list is not recreated.
The publisher reads an existing
artifact again immediately before updating it. The API does not provide a
documented atomic compare-and-swap contract; this check cannot eliminate a human
edit in the interval between that read and the update.

An HTTP 404 from an artifact list or detail read is treated as no existing report.
The publisher tries a fresh upload instead, even when an earlier comment records
an artifact number. It records a warning in the job log and `result.json`, without
adding publishing details to the visible comment. Earlier artifacts remain
untouched, so more than one report of a type can exist while reads are unavailable.
When reads recover, the saved number identifies the latest report to check and
update. A partial publication with an existing comment cannot duplicate uploads
when its reports are unreadable. Other read errors are failures; issue and
comment reads do not use this exception.

A failed artifact write does not repeat successful writes. The missing document
falls back to the comment's collapsed section, and the job records a failure.
Duplicate comments stay one sentence; their failed report remains in the review
files instead. Lost write responses are checked against the server before any
further action, not blindly retried. If a reconciliation read also returns 404,
the write remains unverified and is reported as a failure, not retried.

Comments record whether publication completed and a checksum of the submitted
content. A completed run is a no-op on retry, before any report reads or writes.
An older run cannot replace reports after a newer run has published.

A partial run can repair its missing reports and update its existing fallback
comment when the submitted content is unchanged. The bot-owned comment must still
match the previously verified body; human edits are preserved. A changed input or
an older comment without publication state requires a new run instead.

Every run saves a `triage-reports` workflow artifact with the rendered
`comment.md`, separate report Markdown files, and `result.json`. In staged mode,
these are previews: the publisher makes no API calls, and comment links point
to the adjacent Markdown files. A review index can therefore link to each full
document without creating issue artifacts.

## Local checks

The publisher has no npm dependencies. Its table tests use fake API clients:

```sh
node --test .github/scripts/issue-triage-report.test.cjs
```

The preparation inventory tests use temporary directories and local fake builds:

```sh
node --test .github/scripts/issue-triage-ghmock.test.cjs
```

Command display is covered by the existing `ghmock` run table:

```sh
cd script/ghmock
go test -run '^(TestParseRunArgs|TestRunAll)$' ./...
```

Compile the workflow after changing its instructions or output configuration:

```sh
gh aw compile issue-triage --shellcheck --no-check-update
```

Live tests require an explicitly authorized test repository. Keep trial outputs
staged and verify both the comments and report bodies. A green run does not
establish that a reported bug reproduced, nor that staged artifact writes work
with the production App's permissions.
