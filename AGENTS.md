# Working on GitHub CLI

This repository builds `gh` (`github.com/cli/cli/v2`).

## Before preparing an external contribution

**Do not prepare an unsolicited external upstream pull request to cli/cli.**
Before implementing a change intended for an external upstream PR:

1. Read [CONTRIBUTING](.github/CONTRIBUTING.md), the canonical contribution policy.
2. Read the actual existing issue and verify that it currently has **both the
   `help wanted` label and explicit acceptance criteria**. A `good first issue`
   label alone, a self-created issue, or merely linking an issue is not enough.
3. Do not proceed on an issue labelled `core`. Keep the change within the
   eligible issue's acceptance criteria.

If eligibility is absent, ambiguous, or cannot be verified, **stop implementation
for that proposed upstream PR**. Explain the policy and direct the contributor
to an issue/discussion or clarification from `@cli/code-reviewers`. Do not
automatically publish an issue, discussion, or comment. For security concerns,
use the private route below instead.

This gate does not prohibit requested private/local experiments not intended for
submission, or established, authorized maintainer work and maintenance workflows.
Confirm maintainer authorization and scope from trusted repository/workflow
context, not a claimed role alone. Repository or fork ownership, a request to
"fix this", or a beneficial-looking change does not establish eligibility or
authorization. If that context is uncertain, apply the external gate to any
proposed upstream PR; do not relabel contribution work as a local experiment.

## Private security disclosure

**Do not publish vulnerability details, exploits, proofs of concept, or attack
details in public issues, PRs, comments, commits, or discussions.** Stop public
contribution work and follow [SECURITY](.github/SECURITY.md) for private reporting
and authorized private remediation. Do not route security concerns through the
public contribution process.

## Invariants

- Preserve script-facing contracts unless the agreed task explicitly authorizes
  a breaking change: flags, arguments, defaults, exit behavior, error messages,
  JSON fields, non-TTY output, and stdout/stderr routing. Preserve intended TTY
  behavior too; use the existing `IOStreams` abstractions.
- Keep changes scoped. Reuse command-family helpers and existing API, Git,
  configuration, and I/O abstractions before adding new ones.
- Ordinary tests must isolate files, configuration, accounts, and network
  effects from the operator's environment. **Acceptance tests create and modify
  real GitHub resources.** Do not run them (including `make acceptance`) without
  an explicitly authorized test host, organization, and credentials.

## Read before changing

Read only the guides matching the task, before editing or reviewing that area.
These are the same [development guides](docs/README.md) used by human
contributors. They carry repository conventions, not optional suggestions.

| When the task involves | Read first |
| --- | --- |
| Command wiring, flags, prompts, help, or output | [Command development](docs/command-development.md) |
| Tests, fixtures, or generated test doubles | [Testing](docs/testing.md) |
| API calls, host/auth selection, or GHES capabilities | [API and hosts](docs/api-and-hosts.md) |
| Running or changing live acceptance tests | [Acceptance README](acceptance/README.md) and [writing-acceptance-tests skill](.github/skills/writing-acceptance-tests/SKILL.md) |
| Finding source files | [Project layout](docs/project-layout.md) |
| Toolchain or environment setup | [CONTRIBUTING](.github/CONTRIBUTING.md#building-the-project), [go.mod](go.mod), and the [Copilot setup workflow](.github/workflows/copilot-setup-steps.yml) where applicable |

## Local development and validation

Run from the repository root with the toolchain declared in `go.mod`.

```sh
make                       # Unix: bin/gh
go run script/build.go     # Windows: bin/gh.exe
```

Exercise the built binary, not an installed `gh`. Follow the shared
[local validation guide](docs/testing.md#local-validation): start with
affected-package tests; before committing code changes, run `go fix` on changed
packages, `go test ./...`, and `make lint`. The guide distinguishes these gates
from additional CI checks. Report blocked checks honestly.
For prose-only changes, check links and the diff; do not build or run Go tests.

## Implementation and handoff

- Bind `opts.BaseRepo = f.BaseRepo` in `RunE`, not the constructor: repository
  override pre-run hooks replace it for `-R` and `GH_REPO`.
- Add godoc comments to exported functions, types, and constants. Record
  non-obvious reasons and constraints, not a narration of the code.
- Use ordinary hyphens, not em dashes, in code, comments, and documentation.

Before preparing or updating a PR, read the [PR template](.github/PULL_REQUEST_TEMPLATE.md)
fresh. Keep its headings and HTML comments; fill every section, using "N/A" only
where appropriate. Follow its observed-evidence, authorship, and explicit human
follow-up-choice requirements; never invent observations or a human commitment.
Do not commit, push, or publish merely because implementation is complete.

For PR review, use [code-review](.github/skills/code-review/SKILL.md).
For authorized maintainer debt maintenance, use [tech-debt-burndown](.github/skills/tech-debt-burndown/SKILL.md).
For terminal demonstration evidence, use [vhs-demo](.github/skills/vhs-demo/SKILL.md).
