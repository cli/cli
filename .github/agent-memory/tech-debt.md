# Tech debt burndown: agent memory

Standing corrections for the `tech-debt-burndown` skill. This file is loaded at
the start of every run and is binding. Keep it short: every line here costs
context on every future run.

Add an entry when a run produces knowledge a future run would otherwise have to
rediscover. Consolidate entries that say the same thing.

## Off limits

- Generated code and mocks. See the Never touch section of the skill.
- Removing a feature-detection gate (`// TODO <cleanupIdentifier>`). Whether a
  gate can come out depends on the supported GHES version window, which is not
  discoverable from the repo. Ask.

## Known scale of the linter backlog

Measured 2026-08-06 against `trunk` with `--no-config --default=none
--max-issues-per-linter=0 --max-same-issues=0`, so this repo's exclusions are
*not* applied and these are upper bounds:

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
