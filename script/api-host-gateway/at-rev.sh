#!/usr/bin/env bash
# Run the gateway acceptance subset against an arbitrary revision, by applying
# the harness onto it in a detached worktree. gh is compiled into the test
# binary, so the revision under test has to be built with the harness present.
#
# Usage: HARNESS_COMMIT=<sha> GH_APIHOST_ORG=<org> GH_APIHOST_ORG_TOKEN=<token> \
#          at-rev.sh <revision>
#
# HARNESS_COMMIT: the squashed harness commit to cherry-pick onto the revision.
# GH_APIHOST_ORG: the GitHub organisation the token can create repositories in.
# GH_APIHOST_ORG_TOKEN: a fine-grained PAT for that organisation. Must be set
#   in the environment; the script will not prompt for it.

set -euo pipefail

REV="${1:?usage: at-rev.sh <revision>}"
HARNESS="${HARNESS_COMMIT:?set HARNESS_COMMIT to the squashed harness commit}"

if [ -z "${GH_APIHOST_ORG_TOKEN:-}" ]; then
	printf 'error: GH_APIHOST_ORG_TOKEN must be set in the environment\n' >&2
	exit 1
fi

if [ -z "${GH_APIHOST_ORG:-}" ]; then
	printf 'error: GH_APIHOST_ORG must be set in the environment\n' >&2
	exit 1
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
worktree="$(mktemp -d)/gh-at-$REV"

git -C "$repo_root" worktree add --detach "$worktree" "$REV"
trap 'git -C "$repo_root" worktree remove --force "$worktree"' EXIT

git -C "$worktree" cherry-pick "$HARNESS"

GH_APIHOST_ACCEPTANCE=yes \
	GH_APIHOST_ORG="$GH_APIHOST_ORG" \
	GH_APIHOST_ORG_TOKEN="$GH_APIHOST_ORG_TOKEN" \
	"$worktree/script/api-host-gateway/run.sh"
