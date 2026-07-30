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
    # `search_pull_requests` (find in-scope PRs) and `pull_request_read`, whose
    # `get_check_runs` / `get_status` methods cover CI status and whose
    # `get_comments` method reads the conversation comments used for dedup.
    # `repos` provides list_commits / list_tags / get_release_by_tag for the
    # upstream old->new validation. No `actions` toolset: check runs come from
    # `pull_request_read`, so there is no need to grant workflow/log reads.
    toolsets: [context, repos, pull_requests]
    # Integrity filtering. `approved` is already the default for public repos,
    # but it is stated explicitly because the triager depends on it in both
    # directions:
    #
    #   - It is a real security control. Comments from drive-by accounts
    #     (author_association CONTRIBUTOR / FIRST_TIME_CONTRIBUTOR / NONE) are
    #     dropped by the MCP gateway before the agent sees them, so an arbitrary
    #     GitHub user cannot plant a prompt-injection payload in a comment on a
    #     Dependabot PR, nor forge the dedup marker below. Dependabot itself is a
    #     trusted platform bot and is exempt, so its PR bodies still reach us.
    #
    #   - It would otherwise break dedup. The triage app posts with
    #     author_association NONE, so at `approved` its OWN prior comments would
    #     be filtered out, the head-SHA marker would never be found, and the
    #     workflow would re-comment on every open Dependabot PR every 6 hours.
    #     `trusted-users` promotes the app to `approved` to prevent that.
    #
    # Keep this list in sync with the GitHub App used by safe-outputs below.
    allowed-repos: "all"
    min-integrity: approved
    trusted-users: ["cli-triage[bot]"]

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
  # which is the identity the triager looks for when deduplicating - see
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
---

# Dependabot triage - shared security envelope

Read-only GitHub tooling plus a comment-only safe-output policy for the
Dependabot PR triager. It grants no ability to merge, approve, label, or
otherwise mutate pull requests.
