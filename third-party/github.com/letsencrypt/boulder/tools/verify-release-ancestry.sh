#!/usr/bin/env bash
#
# Usage: verify-release-ancestry.sh <commit hash>
#
# Exits zero if the provided commit is either an ancestor of main or equal to a
# hotfix branch (release-branch-*). Exits 1 otherwise.
#
set -u

if git merge-base --is-ancestor "$1" origin/main ; then
  echo "'$1' is an ancestor of main"
  exit 0
elif git for-each-ref --points-at="$1" "refs/remotes/origin/release-branch-*" | grep -q "^$1.commit.refs/remotes/origin/release-branch-" ; then
  echo "'$1' is equal to the tip of a hotfix branch (release-branch-*)"
  exit 0
else
  echo
  echo "Commit '$1' is neither an ancestor of main nor equal to a hotfix branch (release-branch-*)"
  echo
  exit 1
fi
