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
    # routed through safe-outputs below. `pull_requests` reads the Dependabot
    # PRs, `actions` reads check-run/CI status, `repos` enables the upstream
    # vOLD...vNEW compare used for source-level validation, and `search` finds
    # in-scope PRs and the workflow's own prior triage comment.
    toolsets: [context, repos, pull_requests, actions, search]

# GitHub API domains are always allowed; `defaults` adds only basic
# infrastructure (certs, package mirrors) and NO general web egress. The agent
# validates dependency changes through GitHub's own API, not arbitrary sites.
network: defaults

safe-outputs:
  # Post as the shared triage GitHub App (the same app used by issue-triage).
  # The app mints a short-lived installation token per run and is revoked
  # afterwards, so the workflow's own GITHUB_TOKEN can stay read-only. The app
  # must be granted "Pull requests: write" so it can post PR comments.
  github-app:
    client-id: ${{ secrets.CLI_TRIAGE_APP_CLIENT_ID }}
    private-key: ${{ secrets.CLI_TRIAGE_APP_PRIVATE_KEY }}
  # The ONLY write this workflow can perform is posting a comment. There is
  # deliberately no merge, approve, or label safe-output, so the triager is
  # advisory only and can never auto-merge a pull request.
  add-comment:
    target: "*"                 # a scheduled run has no single triggering item
    max: 50                     # effectively uncapped for one reconcile pass
    hide-older-comments: true   # collapse the superseded triage comment
    footer: true
---

# Dependabot triage - shared security envelope

Read-only GitHub tooling plus a comment-only safe-output policy for the
Dependabot PR triager. It grants no ability to merge, approve, label, or
otherwise mutate pull requests.
