#!/usr/bin/env bash

set -euo pipefail

: "${DEFAULT_BRANCH:?DEFAULT_BRANCH must be set}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"
: "${WORKLIST_PATH:?WORKLIST_PATH must be set}"

workflow="${TRIAGE_WORKFLOW:-dependabot-triage.lock.yml}"
failed=false

if ! active_titles=$(
	for status in queued in_progress waiting; do
		gh api \
			"repos/$GITHUB_REPOSITORY/actions/workflows/$workflow/runs?status=$status&per_page=100" \
			--jq '.workflow_runs[].display_title' ||
			exit 1
	done
); then
	echo "::error::Could not determine whether Dependabot triage runs are active"
	exit 1
fi

if printf '%s\n' "$active_titles" | grep -Fxq 'Dependabot triage PR #'; then
	echo "A broad manual triage run is active; skipping this poll"
	exit 0
fi

while IFS= read -r number; do
	if printf '%s\n' "$active_titles" | grep -Fxq "Dependabot triage PR #$number"; then
		echo "PR #$number: triage is already queued or running, skipping"
		continue
	fi

	echo "Dispatching Dependabot triage for PR #$number"
	if ! gh workflow run "$workflow" \
		--repo "$GITHUB_REPOSITORY" \
		--ref "$DEFAULT_BRANCH" \
		-f "pr_number=$number"; then
		echo "::error::Failed to dispatch Dependabot triage for PR #$number"
		failed=true
	fi
done < <(jq -r '.[].number' "$WORKLIST_PATH")

if [ "$failed" = true ]; then
	exit 1
fi
