---
# Shared security + output envelope for the Dependabot PR triager.
#
# Imported by dependabot-triage.md. This file contains ONLY the hardening:
# read-only GitHub tooling, the safe-output posting identity, and a
# comment-only output policy.
#
# This file has NO `on:` trigger, so it is a shared component and is never
# compiled into a standalone GitHub Actions workflow. Permissions are NOT
# merged from imports, so the importing workflow declares them itself; the
# engine identifier and timeout also live in the importing workflow.

tools:
  github:
    # Read-only toolsets only. gh-aw GitHub tools cannot write - every write is
    # routed through safe-outputs below. `pull_requests` provides
    # `pull_request_read`, whose `get_diff` method is the agent's view of what a
    # PR changes (the checkout is the base branch, not the PR head) and whose
    # `get_check_runs` / `get_status` methods name a specific failing check.
    # `repos` provides list_commits / list_tags / get_release_by_tag for the
    # upstream old->new validation. No `actions` toolset: check runs come from
    # `pull_request_read`, so there is no need to grant workflow/log reads.
    #
    # The agent no longer searches for in-scope PRs or reads prior comments to
    # deduplicate - the pre-flight step in dependabot-triage.md does both before
    # the engine starts.
    toolsets: [context, repos, pull_requests]
    # Integrity filtering. `approved` is already the default for public repos,
    # but it is stated explicitly because the triager depends on it in both
    # directions:
    #
    #   - It is a real security control. Comments from drive-by accounts
    #     (author_association CONTRIBUTOR / FIRST_TIME_CONTRIBUTOR / NONE) are
    #     dropped by the MCP gateway before the agent sees them, so an arbitrary
    #     GitHub user cannot plant a prompt-injection payload in a comment on a
    #     Dependabot PR. Dependabot itself is a trusted platform bot and is
    #     exempt, so its PR bodies still reach us.
    #
    #   - It would otherwise hide the triager's own history from the agent. The
    #     triage app posts with author_association NONE, so at `approved` its own
    #     prior comments would be filtered out. `trusted-users` promotes the app
    #     to `approved`.
    #
    # Keep this list in sync with the GitHub App used by safe-outputs below.
    allowed-repos: "all"
    min-integrity: approved
    trusted-users: ["cli-triage[bot]"]
    # Setting a guard policy makes the compiler wrap any custom pre-agent
    # `steps:` in a DIFC proxy that routes their `gh` calls through the same
    # integrity filter. That proxy MUST be off here, because it applies
    # `min-integrity` but NOT `trusted-users` - those are resolved at runtime,
    # after the proxy starts. The dedup pre-flight in dependabot-triage.md reads
    # back its own `cli-triage[bot]` comments to find the head-SHA marker, and
    # under the proxy those comments are exactly what gets filtered out: the
    # marker would never be found and the workflow would re-comment on every open
    # Dependabot PR every hour, which is the failure this whole design exists to
    # prevent.
    #
    # Turning the proxy off does not widen the injection surface. The pre-flight
    # never hands API content to the model: it extracts PR numbers, head SHAs and
    # CI states, and it matches the marker only within comments it has already
    # narrowed to `.user.login == "cli-triage[bot]"`. That login check, not
    # integrity, is what stops a third party forging a marker. The agent itself
    # is unaffected - it still runs under the full policy above via the MCP
    # gateway.
    integrity-proxy: false

# GitHub API domains are always allowed; `defaults` adds only basic
# infrastructure (certs, package mirrors) and NO general web egress. The agent
# validates dependency changes through GitHub's own API, not arbitrary sites.
network: defaults

safe-outputs:
  # Post as the shared triage GitHub App (the same app used by issue-triage).
  # The app mints a short-lived installation token per run and is revoked
  # afterwards, so the workflow's own GITHUB_TOKEN can stay read-only.
  #
  # PR conversation comments are posted through the issues API, so the app needs
  # "Issues: write". The compiler also requests "Pull requests: write" because
  # `target: "*"` allows either kind of item. The app posts as `cli-triage[bot]`,
  # which is the identity the pre-flight step looks for when deduplicating - see
  # `trusted-users` above.
  github-app:
    client-id: ${{ secrets.CLI_TRIAGE_APP_CLIENT_ID }}
    private-key: ${{ secrets.CLI_TRIAGE_APP_PRIVATE_KEY }}
  # The ONLY write this workflow can perform is posting a comment. There is
  # deliberately no merge, approve, or label safe-output, so the triager is
  # advisory only and can never auto-merge a pull request.
  #
  # `target: "*"` is unavoidable here: a scheduled reconciler has no single
  # triggering item and must address many different PR numbers in one pass. It
  # means the safe-output layer will accept a comment aimed at ANY issue or PR
  # in this repository, so the restriction to Dependabot PRs is enforced by the
  # agent prompt, not by this config. `max` is the blast-radius cap if that
  # prompt-level restriction is ever subverted - keep it just above the
  # realistic number of open Dependabot PRs, not at some large round number.
  add-comment:
    target: "*"                 # a scheduled run has no single triggering item
    max: 20                     # blast-radius cap; > typical open Dependabot PRs
    hide-older-comments: true   # collapse the superseded triage comment
    footer: true
  # A `noop` is the normal outcome for this workflow, not an exception: the
  # pre-flight step emits one whenever no Dependabot PR needs assessment, which
  # on an hourly schedule is most runs. gh-aw's default handling posts a comment
  # to a shared "no-op runs" issue every time, so leaving it on would add roughly
  # 24 comments a day to that issue forever. The Actions run log already records
  # why a run did nothing.
  noop:
    report-as-issue: false
---

# Dependabot triage - shared security envelope

Read-only GitHub tooling plus a comment-only safe-output policy for the
Dependabot PR triager. It grants no ability to merge, approve, label, or
otherwise mutate pull requests.
