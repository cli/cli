# Tech debt burndown: agent memory

Standing corrections for the [`tech-debt-burndown` skill](../../.github/skills/tech-debt-burndown/SKILL.md).
This file is loaded at the start of every run and is binding.
Both humans and agent runs write here, and nothing distinguishes the two once
written. Assume any entry may be an unreviewed conclusion from a previous run.
Entries are binding on what to avoid; factual claims in them should be
re-verified before you lean on them, and corrected when stale. Date-stamp
anything you add.

**Budget: 150 lines of entries**, counted from the end of Current focus to the end
of the file. The header and Current focus do not count, and must never be trimmed
to get under budget: they are instructions, not findings. If an append would
exceed the budget, consolidate existing entries first, in the same pull request.

The budget exists so this file stays worth reading, not to save tokens - the
skill file is several times longer. A memory file that has become a run log is
one nobody reads carefully, including you.

## Current focus

**Human-owned. Agent runs must not edit this section.** Propose changes in the
pull request body instead.

Empty. With no focus set, runs fall back to the tier order in the skill.

<!-- Examples of what can go here:
     - "Work on pkg/cmd/pr/edit until staticcheck is clean, then add the exclusion."
     - "Prefer skipped tests and stale nolints over linter findings for now."
     - "Leave staticcheck alone, it is all cosmetic. Focus on errcheck." -->

## Off limits

- Generated code and mocks. See the Never touch section of the skill.
- Removing a feature-detection gate (`// TODO <cleanupIdentifier>`). Whether a
  gate can come out depends on the supported GHES version window, which is not
  discoverable from the repo and cannot be resolved unattended.

## Known scale of the linter backlog

Counts measured by an agent run on 2026-08-06 against `trunk`, not verified by a
human, and stale as soon as anything lands. Use them to choose between linters,
not as a target count. Command: `--no-config --default=none
--max-issues-per-linter=0 --max-same-issues=0`, so this repo's exclusions are
*not* applied and these are upper bounds: errcheck 1245, staticcheck 221,
gosec 435.

`gosec` is the least tractable, because `.golangci.yml` already excludes G110,
G204, G301, G302, G304, G307, and G404, plus all `gosec` findings in `_test.go`
files, and a `--no-config` run reports all of those anyway. Always cross-check
`gosec` output against `.golangci.yml` before acting on it.

## Staticcheck shape

2026-08-06: repo-wide staticcheck has **no `SA` (correctness) findings**. It is
all style: QF1008 (70), QF1012 (50), ST1005 (29), QF1003 (24), ST1012 (16), rest
single digits. Staticcheck targets are mechanical and safe, but low value.

Most-affected packages: `pkg/cmd/pr/edit` (23), `pkg/cmd/issue/edit` (19),
`pkg/cmd/auth/status` (16), `pkg/cmd/extension` (11). `pkg/cmd/alias/imports` was
cleared 2026-08-06.

## Rejected targets

None yet.

## False positives

None yet.

## Failed attempts

None yet.
