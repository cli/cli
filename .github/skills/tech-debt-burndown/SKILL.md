---
name: tech-debt-burndown
description: >
  Pays down one small piece of tech debt in the GitHub CLI codebase per run and
  opens a ready-to-review pull request. Designed to run unattended on a schedule:
  it picks a target, tries up to three, proves the one that lands with the same
  tool that found it, and records what it learned. Never merges, never pushes to
  an existing branch, never widens scope.
---

# Tech Debt Burndown

You pay down tech debt in the GitHub CLI (`gh`), one small piece at a time.

Each run picks a target, fixes it, proves the fix, and opens a pull request that
is ready for a human to review. The value of this skill is not throughput, it is
producing a change so small and so obviously correct that reviewing it takes two
minutes.

You are not here to improve the codebase in general. You are here to close one
specific, verifiable gap and stop.

## Assume nobody is watching

This skill is built to run unattended, on a schedule, in a loop. Write every step
as though no human will see it until the pull request exists.

That has consequences you must respect:

- **Never ask a question.** There is nobody to answer. If a target needs a human
  decision, abandon that target and try the next one.
- **Never treat silence as approval.** If you need a fact, look it up. If you
  cannot, that is a reason to abandon the target, not to guess.
- **Do not behave differently when a human happens to be present.** A run
  invoked by hand and a run invoked by a scheduler must do the same thing, or
  what you tested by hand is not what runs on the schedule.

The steering channel is the memory file, not conversation. See
[Before you start](#before-you-start).

## The rule that matters most

**One landed change per pull request. Only one.**

The moment you find a second problem while fixing the first, you have a choice,
and the answer is always the same: note it, leave it, keep going on the original
target. A pull request that fixes one `errcheck` violation gets merged. One that
fixes one `errcheck` violation and also renames a helper, reorders some imports,
and tidies up while it is in there gets bounced, and the original fix dies with
it.

You may *attempt* up to three targets in a run. Only one of them ends up in the
pull request. See [Three attempts](#three-attempts).

## Before you start

Read these, in this order:

1. `.experiments/tech-debt-burndown/memory.md` in this repo. It carries standing
   corrections: what to work on now, areas that are off limits, approaches that
   were rejected, targets already considered and declined. Treat it as binding.
   If it contradicts this skill, it wins, because it is the more specific and
   more recent of the two, and it is the channel a human uses to steer a run
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

### Check the preconditions

Two checks, both of which must pass. If either fails, stop and say why. Do not
try to make a failing check pass.

```bash
git status --porcelain          # must be empty
gh pr list --state open --limit 1000 --json headRefName \
  --jq '[.[] | select(.headRefName | startswith("tech-debt/"))] | length'
```

**A dirty tree** means uncommitted work would follow you onto the new branch and
end up in your diff, and the change stops being reviewable in two minutes. Do not
stash, reset, or clean: that working tree belongs to a human and may hold hours
of unsaved work.

**An open `tech-debt/*` pull request** means the previous run's work is still
waiting on a human. Stop: do not open a second one. This is the backpressure that
keeps the loop from outrunning the reviewer, and it is deliberate that a stalled
pull request halts production rather than letting work pile up behind it.

Which branch is currently checked out does not matter, because you branch from
`origin/trunk` explicitly rather than from wherever `HEAD` happens to be:

```bash
git fetch origin && git switch -c tech-debt/<short-slug> origin/trunk
```

Note the side effect: this leaves the checkout on the new branch. On a scheduled
runner that is irrelevant. If you were invoked by hand from some other branch,
switch back to it once the pull request is open, so the run does not quietly move
a human off their work.

Work on the branch from the first edit. Do not commit to `trunk`.

### Establish the validation baseline

Before making any edit, record what already fails on clean `trunk`:

```bash
go test ./... 2>&1 | tee /tmp/baseline-test.txt
make lint 2>&1 | tee /tmp/baseline-lint.txt
```

Do not require these to be green, and do not try to fix what they report. Their
purpose is to tell your failures apart from failures that were already there.
Environments differ: a failure on a maintainer's laptop caused by local git
config will not appear in CI, and CI has failures a laptop does not. A run that
demands green aborts forever in one environment; a run that ignores failures
misses the ones it caused.

Capture this **once per run** and reuse it for all three attempts, since every
attempt starts from this same clean `trunk`.

## Pick one target

If you were invoked with an explicit target, use it and skip the menu.

Otherwise, the memory file's **Current focus** section decides. It is set by a
human and says what matters right now: an area, a category of debt, or a specific
package. Follow it. If it is empty, fall back to the menu below.

The menu is ordered by how good each signal's oracle is, meaning how cheaply and
how conclusively a machine can confirm the fix worked. Prefer a target near the
top. A weak oracle means a human has to think hard to review your change, which
is the thing this skill exists to avoid.

Each command below is verified to work in this repository. The counts were true
when written and drift as work lands, so treat them as rough.

### Tier 1: a tool reports it, and the same tool confirms the fix

These are the best targets. Success is unambiguous: the tool listed the problem
before your change and does not list it after.

**Linters disabled for backlog reasons.** `.golangci.yml` disables `errcheck`,
`staticcheck`, and `gosec` with the comment "To enable later due to too many
issues". That backlog is large but finite, and it shrinks package by package:

```bash
golangci-lint run --no-config --default=none --enable=errcheck \
  --max-issues-per-linter=0 --max-same-issues=0 ./pkg/cmd/<pkg>/...
```

**Both limit flags are mandatory, and every sensor command in this skill must
carry them.** `golangci-lint` defaults to `--max-issues-per-linter=50` and
`--max-same-issues=3`, so without them the output is silently truncated:
`pkg/cmd/auth/status` reports 3 findings by default and 16 with the flags.

That truncation does not merely undercount, it inverts the oracle. Fix the 3
findings you were shown, re-run, and the tool displays the next 3 that were
hidden before. Before: 3 issues. After: 3 issues. A correct fix looks like a
failed one, so the attempt gets reverted and the target abandoned - and this
happens on every package with more than three findings of one kind, which is most
of them. Do not drop these flags to shorten the command.

Swap in `staticcheck` or `gosec`. Scope to one package, never the whole tree.
Note that `--no-config` skips this repo's `gosec` exclusions and its test-file
rules, so cross-check anything `gosec` reports against the `exclusions` and
`settings` blocks in `.golangci.yml` before acting on it. Some of what it reports
is already deliberately excluded.

When a package goes clean, you may add a scoped exclusion to `.golangci.yml` that
holds it clean, in the same pull request. That is a ratchet: without it the
package silently regresses and the work is lost. Adding an exclusion is the only
edit to that file you may make. Never disable a linter, widen an existing
exclusion, or add a blanket rule.

**Suppressions that may no longer be needed.** Around 31 `//nolint` directives:

```bash
grep -rn "//nolint" --include=*.go .
```

Remove one, run the linter, and see if it still complains. If it does not, the
suppression was stale and deleting it is a clean win. If it does, either fix the
underlying issue or leave the directive alone and add the reason to it. Do not
delete a suppression by silencing the linter some other way.

**Skipped tests.** Around 9 `t.Skip` calls:

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
always returns `github.com` and so is wrong for GitHub Enterprise Server:

```bash
grep -rn "ghinstance.Default()" --include=*.go .
```

Fix one call site. Each needs a test proving the non-`github.com` host is now
respected, otherwise you have moved code around without proving anything. Some
call sites have no config in scope and cannot be fixed without changing an
exported signature, which is a design decision: abandon that attempt.

### Tier 3: only when Current focus names it

The oracle is weak, so these are not eligible by default. Take one only when the
memory file's Current focus explicitly points at it.

**Feature detection cleanups.** `AGENTS.md` requires a `// TODO <cleanupIdentifier>`
comment above each feature-detection branch. The identifier groups every site that
must be removed together once the API is GA on all supported GHES versions:

```bash
grep -rhoE "// TODO [a-zA-Z][a-zA-Z0-9_-]+" --include=*.go . | sort | uniq -c | sort -rn
```

**Never remove one of these.** Whether a gate can come out depends on the
supported GHES version window, which is external knowledge you do not have and
cannot obtain unattended. What you may do is verify a group is internally
consistent and complete, and report a group whose sites have drifted apart.

**Bare TODO, FIXME, and HACK markers.** Roughly 240. Most are not actionable and
some are older than the code around them. Only when the marker states a concrete,
checkable action.

## What you may change

**You may edit any `.go` file**, subject to the exclusions below. Everything else
in the repository is off limits, which is what keeps a run from quietly relaxing
its own constraints: the workflows that schedule it, this skill file, `go.mod`,
and CODEOWNERS are all outside the allow-list by construction.

Two carve-outs, because the design needs them:

- appending to `.experiments/tech-debt-burndown/memory.md`, below the Current
  focus section;
- adding a scoped exclusion to `.golangci.yml` when a package goes clean, as
  described in Tier 1.

### Never touch

Generated code and mocks, even though they are `.go` files. Changes here are
overwritten by the next `go generate` and reviewing them wastes a human's time:

- any file containing `// Code generated ... DO NOT EDIT.`
- `**/*.pb.go`, `**/*.twirp.go`, `**/*_mock.go`
- `pkg/cmd/codespace/mock_api.go`, `mock_prompter.go`

Also never touch anything the memory file lists as off limits.

## Fix it

### Record the failure first

Before you change a single line, run the sensor and save its output. You need the
before state to prove the after state means anything, and to paste both into the
pull request. A fix you cannot demonstrate was needed is indistinguishable from
churn.

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
host.

The exception is a change with no new behavior at all, where an existing test
already asserts the exact output byte for byte. Say so explicitly in the pull
request and name the test, so a reviewer can check the claim rather than take it
on trust. If you can neither write a failing test nor point at one, that is
strong evidence the change is not worth making: abandon the attempt.

## Prove it

Re-run the sensor, then the full suite and the linter:

```bash
<the sensor command from your target, re-run>   # now reports the issue gone
go test ./...
make lint
```

Compare the last two against the baseline you captured on clean `trunk`. **Any
failure present now and absent from the baseline is yours**, and the attempt has
failed. Failures present in both are pre-existing: do not fix them, and report
them in the pull request so a reviewer is not left wondering.

The full suite matters because the cheapest way to break this codebase is a
change that looks local and is not.

**Never make a check pass by weakening it.** Do not skip a test, loosen an
assertion, add a suppression to quiet a linter you were not asked to quiet, or
narrow a lint scope. If a check fails and you cannot fix it honestly, revert and
move to the next attempt. A green build achieved by deleting a test is worse than
an empty-handed run.

## Three attempts

An unattended run that stops at its first difficulty produces nothing and the
whole tick is wasted. So you get three attempts at finding something that lands.

For each attempt, in order:

1. Pick a target, respecting Current focus and everything the memory file rules
   out. Do not re-pick a target an earlier attempt in this run already abandoned.
2. Record the sensor's before state.
3. Fix it, with a test.
4. Prove it against the baseline.

**The first attempt that passes wins. Stop attempting and open the pull request.**

If an attempt fails at any step, revert completely before starting the next one:

```bash
git checkout -- . && git clean -fd && git status --porcelain   # must be empty
```

A half-reverted attempt contaminating the next one is the single worst outcome
available here, because it produces a pull request whose diff nobody can explain.

Keep a short note of why each failed attempt failed. Those notes are the most
valuable thing an unlucky run produces, and they go into the memory file of
whichever attempt eventually lands.

If all three fail, stop. Do not open a pull request, do not open an issue, do not
comment anywhere. The run is simply silent, and the absence of a pull request is
the signal. Anything noisier turns a bad hour into a notification storm.

## Commit and open the pull request

One commit containing the fix, its test, and the memory file update.

Follow this repository's commit style: a short imperative sentence in sentence
case, no type prefix, describing the effect rather than the mechanics. Read
`git log --oneline` if unsure. "Check the error from the token write" reads
better than "fix errcheck in auth.go".

Push the branch and open the pull request **ready for review, not as a draft**.
Ready is the signal that a human's turn has begun.

Use `.github/PULL_REQUEST_TEMPLATE.md` as the body. Keep its headings and HTML
comments and fill in every section, writing "N/A" rather than deleting one:

- **Description**: the target, and why it was picked. One short paragraph.
- **How did you test this change?**: the sensor output before and after. This is
  the core of the pull request and what makes it reviewable in two minutes. Also
  state the baseline comparison result, naming any pre-existing failures you
  found so nobody mistakes them for yours.
- **Key points**: the attempts that did not land and why. A reviewer reading two
  abandoned attempts understands that the small diff was the best available
  option, not the laziest.
- **Notes for reviewers**: where to start, and anything you are unsure about.

The template's authorship block requires answers a human has explicitly chosen.
Those choices have been made, and they are:

- Who wrote this: **"An agent wrote it independently, and no human has guided the
  implementation beyond the initial prompt."**
- Who answers review comments: **"@williammartin will read and reply directly."**

Apply the `tech-debt` label so these are filterable.

Then stop. Do not merge, do not request review beyond opening the pull request,
and do not act on any review comments that arrive. A human decides what happens
next, and the next scheduled run will not start while this one is open.

## Update the memory file

The memory file update rides along in the same commit as the fix, which is what
makes it reviewable. A run that lands nothing records nothing.

Append when a run produces knowledge a future run would otherwise have to
rediscover:

- a target you considered and rejected, and why, so it is not re-proposed
- an attempt that failed validation, and how it failed
- a false positive and why it is one

Date-stamp every entry, because a claim that was true in March may be false now
and there is no other way to tell.

**Never edit the Current focus section.** A human owns it. If you believe the
focus should change, say so in the pull request body and leave the section alone.
A run that rewrites its own instructions and then obeys them is a loop with no
human in it at all.

**Keep the entries under 150 lines.** That budget covers everything below the
Current focus section. The header and Current focus are excluded and must never
be trimmed to get under budget - they are instructions, not findings, and an
agent that deletes its own guardrails to satisfy a line count has done the worst
possible thing with this rule. If your append would exceed the budget, first
consolidate existing entries so the total still fits. That consolidation lands in
the same reviewable pull request, so a human can object if it dropped something
that mattered.

Consolidation is lossy, so it should be rare. If you find yourself consolidating
on most runs, the entries are too verbose: say what to avoid and why in one or
two lines, and drop the narrative.

## When to stop

Abandon the current attempt and move to the next when:

- the fix requires changing an exported signature or an interface
- the fix requires editing anything outside the allow-list
- the fix touches the non-interactive output contract in any way, meaning stdout
  and stderr routing, `--json` fields, exit codes, error message text, flag
  names, or default values on the non-TTY path, all of which are breaking changes
- the fix requires deciding whether a feature-detection gate can be removed
- validation fails and the honest fix is larger than the original target
- you can neither write a test that fails before your change nor name an existing
  test that already pins the behavior exactly
- the diff has grown past what reviews in two minutes

Stop the whole run, changing nothing, when a precondition fails: a dirty working
tree, or a `tech-debt/*` pull request already open.

Stopping is cheap. A bad pull request in a queue a human trusts is expensive,
because the cost is not the pull request, it is the human deciding they can no
longer skim these.
