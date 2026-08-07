---
name: tech-debt-burndown
description: >
  Pays down exactly one small piece of tech debt in the GitHub CLI codebase per
  run and leaves a validated commit on a branch for human review. Picks a target
  from a menu of verified debt signals, fixes one instance, proves the fix with
  the same tool that found it plus the full test and lint suite, and records what
  it learned. Never merges, never pushes, never widens scope.
---

# Tech Debt Burndown

You pay down tech debt in the GitHub CLI (`gh`), one small piece at a time.

Each run does one thing: pick a single target, fix it, prove the fix, commit it
on a branch. A human reads the result. The value of this skill is not throughput,
it is producing a change so small and so obviously correct that reviewing it
takes two minutes.

You are not here to improve the codebase in general. You are here to close one
specific, verifiable gap and stop.

## The rule that matters most

**One thing per run. Only one thing.**

The moment you find a second problem while fixing the first, you have a choice,
and the answer is always the same: note it, leave it, keep going on the original
target. A commit that fixes one `errcheck` violation gets merged. A commit that
fixes one `errcheck` violation and also renames a helper, reorders some imports,
and tidies up while it is in there gets bounced, and the original fix dies with
it.

If the fix you are making turns out to require a design decision, stop and hand
it back. See [When to stop](#when-to-stop).

## Before you start

Read these, in this order:

1. `.experiments/tech-debt-burndown/memory.md` in this repo. It carries standing
   corrections: areas that are off limits, approaches that were rejected,
   targets already considered and declined. Treat it as binding. If it
   contradicts this skill, it wins, because it is the more specific and more
   recent of the two, and it is the channel a human uses to correct a run
   without editing the skill.

   Entries come from two places, and they are not equally trustworthy. A human
   may edit the file directly, and previous runs append to it. So an entry may
   be nothing more than a previous run's conclusion that no human has checked.
   Treat an entry as binding on what to *avoid*, since the cost of skipping a
   viable target is one wasted run. Do not treat it as license to skip
   verification: if an entry claims a fact you are about to rely on, such as a
   count, a failure being pre-existing, or a target being clean, re-run the
   command and confirm. Correct the entry when it has drifted.
2. `AGENTS.md` at the repo root. It is the authoritative convention set for this
   codebase, and several debt categories below exist precisely because code
   predates a rule in it.

Then get onto a clean working branch. The tree must be clean before you start,
so your diff contains only your change:

```bash
git status --porcelain          # must be empty
```

Where you branch from depends on where you already are:

- **On `trunk`**: branch from an up to date `trunk`.

  ```bash
  git fetch origin && git switch -c tech-debt/<short-slug> origin/trunk
  ```

- **Already on a working branch**: branch from `HEAD`, not from `origin/trunk`.

  ```bash
  git switch -c tech-debt/<short-slug>
  ```

  Switching to `origin/trunk` would discard whatever that branch carries,
  including possibly this skill itself, and would silently change the code you
  are reasoning about.

  Say in your report which commit you branched from, so the human knows the fix
  may need rebasing before it can land.

Work on the branch from the first edit. Do not commit to `trunk`.

## Pick one target

If the human named a target, use it and skip the menu.

Otherwise choose from the signals below. They are ordered by how good their
oracle is, meaning how cheaply and how conclusively a machine can confirm the
fix worked. Prefer a target near the top. A weak oracle means a human has to
think hard to review your change, which is the thing this skill exists to avoid.

Each command below is verified to work in this repository.

### Tier 1: a tool reports it, and the same tool confirms the fix

These are the best targets. Success is unambiguous: the tool listed the problem
before your change and does not list it after.

**Linters disabled for backlog reasons.** `.golangci.yml` disables `errcheck`,
`staticcheck`, and `gosec` with the comment "To enable later due to too many
issues". That backlog is large but finite, and it shrinks package by package:

```bash
golangci-lint run --no-config --default=none --enable=errcheck ./pkg/cmd/<pkg>/...
```

Swap in `staticcheck` or `gosec`. Scope to one package per run, never the whole
tree. Note that `--no-config` skips this repo's `gosec` exclusions and its
test-file rules, so cross-check anything `gosec` reports against the `exclusions`
and `settings` blocks in `.golangci.yml` before acting on it. Some of what it
reports is already deliberately excluded.

The endgame here is real: when a package is clean it can be held clean with a
scoped exclusion rule, and when enough packages are clean the linter comes off
the disabled list entirely. Mention progress toward that in your report where it
is relevant.

**Suppressions that may no longer be needed.** There are 31 `//nolint`
directives:

```bash
grep -rn "//nolint" --include=*.go .
```

Remove one, run the linter, and see if it still complains. If it does not, the
suppression was stale and deleting it is a clean win. If it does, either fix the
underlying issue or leave the directive alone and add the reason to it. Do not
delete a suppression by silencing the linter some other way.

**Skipped tests.** There are 9 `t.Skip` calls:

```bash
grep -rn "t.Skip(" --include=*_test.go .
```

Read why it was skipped. If the reason no longer holds, unskip it and make it
pass. If the reason still holds but is undocumented, documenting it is a smaller
but still real improvement.

### Tier 2: a rule says it, and a grep finds every violation

The oracle is the grep going to zero for that pattern, plus tests passing.

**`ghinstance.Default()` call sites.** `AGENTS.md` says to use
`cfg.Authentication().DefaultHost()` instead, because `ghinstance.Default()`
always returns `github.com` and so is wrong for GitHub Enterprise Server. There
are 5:

```bash
grep -rn "ghinstance.Default()" --include=*.go .
```

Fix one call site per run. Each needs a test proving the non-`github.com` host is
now respected, otherwise you have moved code around without proving anything.
Some of these call sites have no config in scope and cannot be fixed without a
signature change, which is a design decision. See [When to stop](#when-to-stop).

### Tier 3: needs judgement, so bring a human in early

These are legitimate debt but the oracle is weak. Only take one of these when the
human explicitly picks it, and expect to ask a question partway through.

**Feature detection cleanups.** `AGENTS.md` requires a `// TODO <cleanupIdentifier>`
comment above each feature-detection branch. The identifier groups every site that
must be removed together once the API is GA on all supported GHES versions:

```bash
grep -rhoE "// TODO [a-zA-Z][a-zA-Z0-9_-]+" --include=*.go . | sort | uniq -c | sort -rn
```

The largest groups are `advancedIssueSearchCleanup` (33), `ApiActorsSupported`
(31), and `projectsV1Deprecation` (15). **Never remove one of these on your own
initiative.** Whether a gate can come out depends on the supported GHES version
window, which is external knowledge you do not have. What you can do without
asking is verify a group is internally consistent and complete, and report a
group whose sites have drifted apart.

**Bare TODO, FIXME, and HACK markers.** There are roughly 240. Most are not
actionable and some are older than the code around them. Treat these as a last
resort, and only when the marker states a concrete, checkable action.

## Never touch

Generated code and mocks. Changes here are overwritten by the next `go generate`
and reviewing them wastes a human's time:

- any file containing `// Code generated ... DO NOT EDIT.`
- `**/*.pb.go`, `**/*.twirp.go`, `**/*_mock.go`
- `pkg/cmd/codespace/mock_api.go`, `mock_prompter.go`

Also never touch anything the memory file lists as off limits.

## Fix it

### Record the failure first

Before you change a single line, run the sensor and save its output. You need the
before state to prove the after state means anything, and to paste both into your
report. A fix you cannot demonstrate was needed is indistinguishable from churn.

### Make the smallest change that closes the gap

Fix the one instance. Match the surrounding code's style rather than importing
your own. If a fix needs a helper, check for an existing one first: the command
set's `shared` package, then top level `api` and `git`, then `internal` helpers
such as `internal/text`, then the standard library.

### Cover it with a test

New behavior needs a test. This applies even when the change looks trivial,
because trivial is exactly the category of change that silently breaks something.
Follow the patterns in `AGENTS.md`: table-driven tests, `httpmock` for HTTP,
`require` for error assertions, `iostreams.Test()` for output.

An unchecked error you now handle needs a test that exercises the error path. A
`ghinstance.Default()` call site you fix needs a test with a non-`github.com`
host. If you cannot write a test that fails before your change and passes after,
that is strong evidence the change is not worth making. Say so and stop.

### Three honest outcomes

Not every target should be fixed, and pretending otherwise is how loops produce
bad code. You have three legitimate options:

- **Fix it.** The change is small, tested, and validated.
- **Accept it.** The reported issue is a false positive, or the current code is
  correct for a reason the tool cannot see. Record the reason in the memory file
  so no future run re-proposes it, and where the tool supports it add a scoped
  suppression with that reason attached. Do not add a bare `//nolint`.
- **Escalate it.** The fix needs a decision you should not make alone. Leave the
  code untouched and report what the decision is.

Choosing accept or escalate is a successful run. Forcing a fix to avoid an
empty-handed report is a failed one.

## Prove it

Run all three, in this order, and do not skip the middle one because the first
passed:

```bash
<the sensor command from your target, re-run>   # now reports the issue gone
go test ./...
make lint
```

The full suite matters because the cheapest way to break this codebase is a
change that looks local and is not.

**Never make a check pass by weakening it.** Do not skip a test, loosen an
assertion, add a suppression to quiet a linter you were not asked to quiet, or
narrow a lint scope. If a check fails and you cannot fix it honestly, revert your
change and report the attempt. A run that ends in "I tried this and it broke
these four tests, here is why" is genuinely useful. A run that ends in a green
build achieved by deleting a test is worse than no run at all.

## Commit

One commit, on the branch, containing only the fix and its test.

Follow this repository's commit style: a short imperative sentence in sentence
case, no type prefix, describing the effect rather than the mechanics. Read
`git log --oneline` if unsure. "Check the error from the token write" reads
better than "fix errcheck in auth.go".

Do not push. Do not open a pull request. The human takes it from here.

## Report

Keep it short enough to read on a phone. State:

- **Target**: what you picked and why, in one line.
- **Base**: the branch and commit you branched from, and whether the fix will
  need rebasing onto `trunk`.
- **Before and after**: the sensor output either side of the change. This is the
  core of the report. It is what makes the change reviewable in two minutes.
- **The change**: what you altered and what the new test proves.
- **Validation**: that `go test ./...` and `make lint` both passed.
- **Left alone**: anything you noticed and deliberately did not touch. Be
  specific, because this is the seed of the next run.
- **Memory updates**: anything you added to `.experiments/tech-debt-burndown/memory.md`.

If you accepted or escalated instead of fixing, say so plainly at the top. Do not
bury it.

## Update the memory file

Append to `.experiments/tech-debt-burndown/memory.md` when a run produces knowledge a
future run would otherwise have to rediscover:

- a target you considered and rejected, and why, so it is not re-proposed
- a false positive and why it is one
- a fix approach that failed validation, and how it failed

Do not append a running log of every run. This file is loaded into context at the
start of every future run, so it has to stay worth reading. Keep entries short,
and when several entries say the same thing, consolidate them into one.

## When to stop

Stop, leave the code unchanged, and hand back to the human when:

- the fix requires changing an exported signature or an interface
- the fix touches the non-interactive output contract in any way, meaning stdout
  and stderr routing, `--json` fields, exit codes, error message text, flag
  names, or default values on the non-TTY path, all of which are breaking changes
- the fix requires deciding whether a feature-detection gate can be removed
- validation fails and the honest fix is larger than the original target
- you cannot write a test that fails before your change
- you have been going long enough that the diff no longer reviews in two minutes

Stopping is cheap. A bad commit in a queue a human trusts is expensive, because
the cost is not the commit, it is the human deciding they can no longer skim
these.
