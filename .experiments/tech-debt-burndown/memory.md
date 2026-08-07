# Tech debt burndown: agent memory

Standing corrections for the [`tech-debt-burndown` skill](../../.github/skills/tech-debt-burndown/SKILL.md).
This file is loaded at the start of every run and is binding. Keep it short:
every line here costs context on every future run.

Both humans and agent runs write here, and nothing distinguishes the two once
written. Assume any entry may be an unreviewed conclusion from a previous run.
Entries are binding on what to avoid; factual claims in them should be
re-verified before you lean on them, and corrected when stale.

Add an entry when a run produces knowledge a future run would otherwise have to
rediscover. Consolidate entries that say the same thing.

## Off limits

- Generated code and mocks. See the Never touch section of the skill.
- Removing a feature-detection gate (`// TODO <cleanupIdentifier>`). Whether a
  gate can come out depends on the supported GHES version window, which is not
  discoverable from the repo. Ask.

## Known scale of the linter backlog

Counts measured by an agent run on 2026-08-06 against `trunk`, not verified by a
human, and stale the moment anyone lands a fix. Use them to choose between
linters, not as a target count. Command:
`--no-config --default=none --max-issues-per-linter=0 --max-same-issues=0`, so
this repo's exclusions are *not* applied and these are upper bounds:

| Linter | Issues |
| --- | --- |
| errcheck | 1245 |
| staticcheck | 221 |
| gosec | 435 |

`staticcheck` is the most tractable of the three. `gosec` is the least, because
`.golangci.yml` already excludes G110, G204, G301, G302, G304, G307, and G404,
plus all `gosec` findings in `_test.go` files, and a `--no-config` run will
report all of those anyway. Always cross-check `gosec` output against
`.golangci.yml` before acting on it.

## Rejected targets

None yet.

## False positives

None yet.

## Failed approaches

None yet.

## Baseline noise on this machine

Two failures are pre-existing and unrelated to any fix; confirmed against a
clean tree. Do not try to fix them and do not treat them as validation failures:

- `make lint` reports 3 `govet` issues in vendored/toolchain source.
- `git/...` tests fail with `safe.bareRepository is 'explicit'`, a local git
  config setting, not a code defect.

## Staticcheck shape

Repo-wide staticcheck has **no `SA` (correctness) findings**. It is all style:
QF1008 (70), QF1012 (50), ST1005 (29), QF1003 (24), ST1012 (16), rest single
digits. So staticcheck targets are mechanical and safe, but pick a package that
goes fully to zero rather than fixing scattered instances.

Packages with the most staticcheck issues: `pkg/cmd/pr/edit` (23),
`pkg/cmd/issue/edit` (19), `pkg/cmd/auth/status` (16), `pkg/cmd/extension` (11).
`pkg/cmd/alias/imports` is now clean (2026-08-06).
