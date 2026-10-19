#!/usr/bin/env bash

set -euo pipefail

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"

set_output() {
	if [ -n "${GITHUB_OUTPUT:-}" ]; then
		printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"
	fi
}

write_noop() {
	if [ -n "${GH_AW_SAFE_OUTPUTS:-}" ]; then
		mkdir -p "$(dirname "$GH_AW_SAFE_OUTPUTS")"
		printf '%s\n' "$1" >>"$GH_AW_SAFE_OUTPUTS"
	fi
}

select_worklist() {
	local worklist single prs jq_pending ready work needs_go row entry number head assessed files count

	worklist="${WORKLIST_PATH:-${RUNNER_TEMP:-/tmp}/gh-aw/dependabot-worklist.json}"
	mkdir -p "$(dirname "$worklist")"

	single=""
	if [ -n "${PR_NUMBER_INPUT:-}" ]; then
		if printf '%s' "$PR_NUMBER_INPUT" | grep -qE '^[1-9][0-9]*$'; then
			single="$PR_NUMBER_INPUT"
			echo "Dispatch input restricts this run to PR #$single"
		else
			echo "Ignoring non-numeric pr_number input"
			printf '[]\n' >"$worklist"
			set_output "count" "0"
			set_output "needs_go" "false"
			set_output "reason" "invalid_input"
			set_output "worklist_path" "$worklist"
			write_noop '{"type":"noop","message":"pr_number input was not a positive integer"}'
			return
		fi
	fi

	prs=$(gh pr list --repo "$GITHUB_REPOSITORY" --state open \
		--author app/dependabot --limit 100 \
		--json number,headRefOid,statusCheckRollup)

	# gh truncates silently at --limit, and the listing order is stable, so
	# anything past the cap would never be reached on a later run either.
	if [ "$(printf '%s' "$prs" | jq length)" -ge 100 ]; then
		echo "::warning::Open Dependabot PRs hit the 100 listing cap; any beyond it are not being triaged."
	fi

	if [ -n "$single" ]; then
		prs=$(printf '%s' "$prs" | jq --argjson n "$single" '[.[] | select(.number == $n)]')
	fi

	# statusCheckRollup mixes CheckRun (.status) and StatusContext (.state)
	# shapes. A null rollup means the checks could not be read, so fail closed.
	# shellcheck disable=SC2016
	jq_pending='
	  def pending:
	    if has("status") then (.status != "COMPLETED")
	    else ((.state // "SUCCESS") as $s | $s == "PENDING" or $s == "EXPECTED")
	    end;
	  def pending_names:
	    if .statusCheckRollup == null then ["<check status unreadable>"]
	    else [.statusCheckRollup[] | select(pending) | (.name // .context // "unnamed")]
	    end;
	'

	ready=$(printf '%s' "$prs" | jq -c "$jq_pending"'
	  [ .[]
	    | select((pending_names | length) == 0)
	    | {number: .number, head_sha: .headRefOid} ]')

	printf '%s' "$prs" | jq -r "$jq_pending"'
	  .[]
	  | . as $pr
	  | pending_names
	  | select(length > 0)
	  | "PR #\($pr.number): skipped, checks still pending: \(join(", "))"'

	echo "PRs with terminal CI: $(printf '%s' "$ready" | jq length)"

	work='[]'
	needs_go=false
	while IFS= read -r row; do
		entry=$(printf '%s' "$row" | base64 --decode)
		number=$(printf '%s' "$entry" | jq -r '.number')
		head=$(printf '%s' "$entry" | jq -r '.head_sha')

		# shellcheck disable=SC2016
		assessed=$(gh api "repos/$GITHUB_REPOSITORY/issues/$number/comments" --paginate \
			--jq '.[] | select(.user.login == "cli-triage[bot]") | .body' |
			grep -oE '_Assessed at head commit `[0-9a-f]{40}`\._' |
			tail -1 | grep -oE '[0-9a-f]{40}' || true)

		if [ "$assessed" = "$head" ]; then
			echo "PR #$number: already assessed at $head, skipping"
			continue
		fi

		echo "PR #$number: needs assessment (head $head, last assessed '${assessed:-none}')"
		work=$(printf '%s' "$work" | jq -c --argjson entry "$entry" '. + [$entry]')

		# Only the agentic workflow consumes this output. Treat an unreadable
		# file list as possible Go work so missing evidence cannot increase
		# confidence.
		files=$(gh pr view "$number" --repo "$GITHUB_REPOSITORY" --json files \
			--jq '.files[].path' 2>/dev/null) || files="go.mod"
		if printf '%s\n' "$files" | grep -qE '^(go\.mod|go\.sum)$'; then
			needs_go=true
		fi
	done < <(printf '%s' "$ready" | jq -r '.[] | @base64')

	printf '%s\n' "$work" >"$worklist"
	count=$(printf '%s' "$work" | jq length)
	echo "Work list: $count PR(s) -> $worklist"
	echo "Go reachability evidence needed: $needs_go"

	set_output "count" "$count"
	set_output "needs_go" "$needs_go"
	set_output "reason" "$([ "$count" -eq 0 ] && echo no_work || echo ready)"
	set_output "worklist_path" "$worklist"

	if [ "$count" -eq 0 ]; then
		write_noop '{"type":"noop","message":"No Dependabot PRs need triage: all open PRs are already assessed at their current head commit, or their CI is still pending."}'
	fi
}

dispatch_worklist() {
	local workflow failed active_titles number

	: "${DEFAULT_BRANCH:?DEFAULT_BRANCH must be set}"
	: "${WORKLIST_PATH:?WORKLIST_PATH must be set}"

	workflow="${TRIAGE_WORKFLOW:-dependabot-triage.lock.yml}"
	failed=false

	if ! active_titles=$(
		for status in queued in_progress waiting pending; do
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
		return
	fi

	while IFS= read -r number; do
		if printf '%s\n' "$active_titles" | grep -Fxq "Dependabot triage PR #$number"; then
			echo "PR #$number: triage is already queued or active, skipping"
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
}

case "${1:-}" in
select)
	select_worklist
	;;
dispatch)
	dispatch_worklist
	;;
*)
	echo "usage: $0 {select|dispatch}" >&2
	exit 2
	;;
esac
