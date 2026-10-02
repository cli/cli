# Maintaining and evaluating the skill

This file is for authors and reviewers, not normal exercise runs.

## Design choices

- The entry skill is a concise router. Its descriptions state both capability
  and activation context; detailed instructions are loaded only when needed.
- Every internal module and reference is linked directly from the entry file.
  Directory nesting does not require a chain of reference reads.
- Exact cases have low execution freedom; goals have bounded exploratory
  freedom. Changing execution strategy does not change fidelity requirements.
- Recording cases cover one behavior, with at most one exercise invocation.
  The agent actually executes preparation, exercise, and immediate validation
  for each case before the next. Exact supplied steps and narrower coverage
  remain authoritative; conflicts need clarification, not silent restructuring.
- Normal delivery uses one media-only session video, even for one case.
  Existing manifest titles and phases drive presentation; fidelity comes from
  exercise contracts and is recorded with overview timing in rendered metadata.
  Long overviews gain readable pages at 10 seconds per page. Progress follows
  the complete final timeline, including overviews and reading holds.
- Invocation display comes from immutable recorded executable/argument data,
  not extra terminal input. Phase text accompanies color; expectation outcomes,
  not exit-code positivity, determine validation status. Explicit unannotated
  real-time output stays free of overlays.
- Runtime timestamps support rejecting nonchronological, overlapping, or reused
  captures. Legacy captures remain readable with an explicit
  `execution order unverified` warning. Presentation must not rewrite original
  contracts, captures, or historical execution order.
- Go performs deterministic validation, policy enforcement, capture, and rendering.
  Node is only the mechanical adapter to the Tuistory API. Agents normally run
  them through documented interfaces rather than reading all implementation code.
- Contracts, readiness receipts, controller sources, observed states, and
  case results are verifiable intermediate artifacts.
- The shared terminal package supplies the production bridge for both execution
  and readiness probing. Recording types provide common receipt, output, and
  terminal-data definitions; original contract and terminal JSON remain intact.
- Profile-directory setup is shared, while each caller chooses its additional
  environment variables explicitly.
- No broad shell-tool preapproval is declared in skill metadata. Installation
  and resource effects require their own appropriate authorization.
- Internal modules are bundled resources, not independently required installs
  or background agents. `user-invocable` is a host-specific visibility hint,
  not a safety or portability assumption.

## Local checks

From the repository root:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  <selected-go> -C .github/skills/cli-exercise/tool test -mod=readonly ./...
bin/gh skill publish --dry-run .github
```

The last command validates metadata only. Do not omit `--dry-run`: publishing,
committing, or pushing is a separate user decision.

Unit tests must use temporary fixtures and fake dependency/process operations.
They must not use the operator's account, home configuration, or live network.
Keep actual runtime/media smoke tests separate and disclose their prerequisites.
No dependency installation is authorized merely by running a validation command.

Use `CLI_EXERCISE_TEST_HELPER` and `CLI_EXERCISE_TEST_RECEIPT` to select an
already-built helper and a genuine ready receipt for native PTY checks. The
Go terminal tests use the receipt for mechanical operations and lifecycle;
execution tests also use the helper for exact and hybrid cases through both
file/stdin clients.
The synthetic target is the Go test binary running in fixture mode, not a
JavaScript program or `gh`. The readiness probe also uses a Go fixture.
All executable tests and fixtures are Go; no separate Node test runner is needed.
The behavioral-rubric JSON is instruction evaluation data, not an executable
test or fixture.
No tool or package installation is permitted as an implicit test setup step.
The evidence package can replay an existing test-owned capture with
`CLI_EXERCISE_EVIDENCE_RUN` and `CLI_EXERCISE_EVIDENCE_PREFLIGHT` pointing to
that capture and its ready tools. It adds a separate rendering, not a new target
invocation. Without those selections, its optional native case is skipped.
Native integration has been exercised on macOS; other platforms require their
own successful capability checks and are not claimed as validated.

## Agent-behavior evaluations

The rubrics in [skill-evaluations.json](../tests/skill-evaluations.json) cover
exact-case fidelity, missing authority, installation refusal, mixed inputs,
progressive resource loading, one-behavior case boundaries, immediate per-case
validation, session presentation, and the distinction between case and media
status. They also cover exact-case conflicts, long-list pagination, truthful
outcome styling, no-overlay requests, and chronology limitations.
They are evaluation inputs, not executable tests or an assertion that a model
evaluation ran. Validate their JSON structure and review instruction consistency
when changing the rubric; do not add a separate test language or CI workflow.

Test these with a fresh agent and isolated synthetic tools where practical.
Observe which files it loads and which tool calls it makes. Compare against the
rubric and the skill-less baseline, then make the smallest instruction change
that addresses an observed failure. Record the model and actual coverage;
success with one model is not evidence for every model.

## Research basis

- [Agent Skills specification](https://agentskills.io/specification): metadata,
  concise activation instructions, on-demand resources, and direct file references.
- [Skill authoring best practices](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices):
  degrees of freedom, progressive disclosure, executable utilities, and evaluation.
- [GitHub Copilot CLI skills](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills):
  packaging, referenced resources, activation, and caution around pre-approved tools.

Apply guidance within this repository's stricter error, privacy, and approval
rules. Do not turn a missing file, failed command, or denied permission into a
success-shaped default.
