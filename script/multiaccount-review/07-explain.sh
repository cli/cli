#!/bin/bash
# Replay: bash 07-explain.sh --gh /absolute/gh --case precedence run-all
set -euo pipefail
LAYER=07-explain.sh
source "$(cd "$(dirname "$0")" && pwd -P)/common.sh"
if [[ "${1:-}" = --_isolated ]]; then shift; else parse precedence "$@"; fi
inner_parse "$@"
[[ "$CASE" = precedence || "$CASE" = missing-account ]] || die "unknown case"

setup() {
  base_setup
  identity tree identity-fixture-bob 'Tree Identity' tree@example.invalid
  identity project identity-fixture-alice 'Project Identity' project@example.invalid
  "$GH" auth use tree --directory "$F/tree"
  cd "$F/tree/project"
  "$GH" auth use project
  git config --local user.name 'Local Committer'
  git config --local user.email local@example.invalid
  if [[ "$CASE" = missing-account ]]; then
    accounts bob-only
    printf 'Removed only synthetic Alice from fixture hosts.yml; selection remains.\n'
  else
    printf 'Repository identity overrides tree identity; local Git attribution is independent.\n'
  fi
}
exercise() {
  cd "$F/tree/project"
  "$GH" auth use --explain --hostname "$HOST"
}
validation() {
  cd "$F/tree/project"
  local query
  if [[ "$CASE" = precedence ]]; then
    query='.selection.identity.name == "project" and .selection.identity.account == "identity-fixture-alice" and .selection.identity.gitUserEmail == "project@example.invalid" and .selection.selected.repository != null and .selection.superseded[0].binding.identity == "tree" and .selection.superseded[0].reason == "repository choices take precedence" and .attribution.status == "observed" and .attribution.email.value == "local@example.invalid" and (.attribution.email.origin | contains(".git/config")) and (.configuration | any(.targetObserved)) and .authentication.api.status == "declared"'
    printf '$ gh auth use --explain --json selection,attribution,configuration,authentication --jq <predicate>\n'
    expect "$("$GH" auth use --explain --hostname "$HOST" \
      --json selection,attribution,configuration,authentication --jq "$query")" true \
      'JSON: repository beats directory; identity default differs from observed local email'
    printf '$ git config --show-origin --get user.email\n'
    git config --show-origin --get user.email
    git -c commit.gpgsign=false commit --allow-empty -qm 'Offline attribution check'
    printf '$ git log -1 --format=author-and-committer-emails\n'
    git log -1 '--format=%ae | %ce'
    expect "$(git log -1 '--format=%ae|%ce')" 'local@example.invalid|local@example.invalid' \
      'independent Git commit uses local attribution, not the identity default'
  else
    query='.selection.identity.name == "project" and .authentication.api.status == "blocked" and .authentication.https.status == "blocked" and (.warnings | length > 0) and (.authentication.api.detail | contains("not signed in"))'
    expect "$("$GH" auth use --explain --hostname "$HOST" \
      --json selection,authentication,warnings --jq "$query")" true \
      'JSON retains selected identity and marks missing-account authentication blocked'
    "$GH" auth use --explain --hostname "$HOST" --json authentication,warnings \
      --jq '{api: .authentication.api.status, https: .authentication.https.status, warnings: .warnings}'
    expect "$("$GH" auth use --explain --hostname "$HOST" --json selection --jq .selection.identity.name)" project \
      'inspection did not replace the scoped identity with the active account'
  fi
  safe_network
}
run_phases
