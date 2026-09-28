#!/usr/bin/env bash
# Black box test for per-host api_host routing, run inside the container that
# run.sh starts. See README.md for why this needs a container.
#
# The test proves three things:
#   1. With api_host set, gh sends its API traffic to the gateway and still gets
#      real answers from github.com, even though api.github.com is blackholed.
#   2. Without api_host, gh goes straight to api.github.com and the gateway sees
#      nothing.
#   3. With the blackhole in place and no api_host, gh cannot reach GitHub at
#      all, so phase 1 cannot be passing by accident.

set -uo pipefail

GATEWAY_HOST=${GH_APIHOST_GATEWAY_HOST:-gh-gateway.internal}
GATEWAY_IP=127.0.0.2
UPSTREAM_HOST=api.github.com
REPO=${GH_APIHOST_REPO:-/src}
WORK=${GH_APIHOST_WORK:-/tmp/api-host-gateway}
EXPECTED_LOGIN=${GH_APIHOST_EXPECTED_LOGIN:-}
TOKEN=${GH_APIHOST_TOKEN:-}
ORG_TOKEN=${GH_APIHOST_ORG_TOKEN:-}

CONFIG_DIR="$WORK/ghconfig"
LOG="$WORK/gateway.jsonl"
BUNDLE="$WORK/bundle.pem"
RESULTS="$WORK/results.tsv"

FAILURES=0
SUBSET_REDS=0
SUBSET_RED_NAMES=
SUBSET_NOMATCHES=0
SUBSET_NOMATCH_NAMES=
GATEWAY_PID=

# PHASE labels every recorded result so the summary can be read phase by phase.
# Set it immediately before the assertions belonging to a phase.
PHASE=0

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

# record <name> <PASS|FAIL|NOMATCH> appends one line to the results ledger that
# the summary prints. Named results matter because a commit that fixes one call
# site should be visible as that specific assertion turning green.
record() {
	printf '%s\t%s\t%s\n' "$PHASE" "$2" "$1" >>"$RESULTS"
}

pass() {
	printf '  ok   %s\n' "$1"
	record "$1" PASS
}

fail() {
	printf '  FAIL %s\n' "$1"
	FAILURES=$((FAILURES + 1))
	record "$1" FAIL
}

heading() {
	printf '\n== %s\n' "$1"
}

cleanup() {
	if [ -n "$GATEWAY_PID" ]; then
		kill "$GATEWAY_PID" 2>/dev/null
	fi
}

# --- setup -------------------------------------------------------------------

[ -n "$TOKEN" ] || die "GH_APIHOST_TOKEN is required"
[ "$(id -u)" = "0" ] || die "must run as root to bind port 443 and edit /etc/hosts"

mkdir -p "$WORK" "$CONFIG_DIR"
chmod 700 "$CONFIG_DIR"
rm -f "$LOG" "$WORK/ready"
: >"$RESULTS"

heading "Building gh and the gateway"
# The repository may be mounted from a git worktree, whose .git file points
# outside the mount, so VCS stamping cannot work here.
(cd "$REPO" && go build -buildvcs=false -o "$WORK/gh" ./cmd/gh) || die "building gh"
(cd "$REPO" && go build -buildvcs=false -o "$WORK/gateway" ./script/api-host-gateway/gateway) || die "building the gateway"

# Resolve the upstream before the blackhole goes in, so the gateway can keep
# reaching GitHub once gh no longer can.
UPSTREAM_IP=$(getent ahostsv4 "$UPSTREAM_HOST" | awk 'NR==1 {print $1}')
[ -n "$UPSTREAM_IP" ] || die "could not resolve $UPSTREAM_HOST"

heading "Starting the gateway"
"$WORK/gateway" \
	-listen "$GATEWAY_IP:443" \
	-gateway-host "$GATEWAY_HOST" \
	-upstream-host "$UPSTREAM_HOST" \
	-upstream-addr "$UPSTREAM_IP:443" \
	-ca-out "$WORK/ca.pem" \
	-log "$LOG" \
	-ready "$WORK/ready" >"$WORK/gateway.stderr" 2>&1 &
GATEWAY_PID=$!
trap cleanup EXIT

for _ in $(seq 1 100); do
	[ -f "$WORK/ready" ] && break
	sleep 0.1
done
[ -f "$WORK/ready" ] || die "gateway did not start: $(cat "$WORK/gateway.stderr")"
printf 'gateway pid %s, upstream %s at %s\n' "$GATEWAY_PID" "$UPSTREAM_HOST" "$UPSTREAM_IP"

# Go on Linux honours SSL_CERT_FILE, which is the whole reason this test runs in
# a container. Keep the real roots so anything else gh talks to still works.
cat /etc/ssl/certs/ca-certificates.crt "$WORK/ca.pem" >"$BUNDLE" || die "building the trust bundle"

grep -q "$GATEWAY_HOST" /etc/hosts || printf '%s %s\n' "$GATEWAY_IP" "$GATEWAY_HOST" >>/etc/hosts

# The authenticated login is both asserted against and written into hosts.yml,
# so derive it from the token rather than hardcoding a default that would only
# be correct for one developer. This runs before the blackhole goes up, so
# api.github.com is still reachable, and it deliberately does not use gh: the
# whole point of the test is that gh's routing is what is under examination.
if [ -z "$EXPECTED_LOGIN" ]; then
	[ -n "$TOKEN" ] || die "GH_APIHOST_TOKEN is required to determine the authenticated login"
	EXPECTED_LOGIN=$(curl -fsS \
		-H "Authorization: token $TOKEN" \
		-H "Accept: application/vnd.github+json" \
		https://api.github.com/user |
		sed -n 's/.*"login"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
	[ -n "$EXPECTED_LOGIN" ] ||
		die "could not determine the authenticated login; set GH_APIHOST_EXPECTED_LOGIN"
fi

# --- helpers -----------------------------------------------------------------

# write_config yes|no [token] controls whether hosts.yml carries the api_host
# override. The optional second argument sets the oauth_token; defaults to
# $TOKEN so existing single-argument call sites are unchanged.
write_config() {
	local tok="${2:-$TOKEN}"
	{
		printf '%s:\n' "github.com"
		printf '    user: %s\n' "$EXPECTED_LOGIN"
		printf '    oauth_token: %s\n' "$tok"
		printf '    git_protocol: https\n'
		if [ "$1" = "yes" ]; then
			printf '    api_host: %s\n' "$GATEWAY_HOST"
		fi
		printf '    users:\n'
		printf '        %s:\n' "$EXPECTED_LOGIN"
		printf '            oauth_token: %s\n' "$tok"
	} >"$CONFIG_DIR/hosts.yml"
	chmod 600 "$CONFIG_DIR/hosts.yml"
}

# /etc/hosts is a bind mount in a container, so it can only be rewritten in
# place. Appending and truncating work, renaming a temp file over it does not.
blackhole_on() {
	grep -q "^127.0.0.1 $UPSTREAM_HOST\$" /etc/hosts ||
		printf '127.0.0.1 %s\n' "$UPSTREAM_HOST" >>/etc/hosts
}

blackhole_off() {
	local remaining
	remaining=$(grep -v "^127.0.0.1 $UPSTREAM_HOST\$" /etc/hosts)
	printf '%s\n' "$remaining" >/etc/hosts
}

run_gh() {
	env -u GH_TOKEN -u GITHUB_TOKEN -u GH_HOST -u GH_ENTERPRISE_TOKEN \
		GH_CONFIG_DIR="$CONFIG_DIR" \
		SSL_CERT_FILE="$BUNDLE" \
		GH_NO_UPDATE_NOTIFIER=1 \
		"$WORK/gh" "$@"
}

# expect_gh <description> <expected stdout> <gh args...>
expect_gh() {
	local desc=$1 expected=$2
	shift 2

	local out rc
	out=$(run_gh "$@" 2>"$WORK/stderr.txt")
	rc=$?

	if [ "$rc" -ne 0 ]; then
		fail "$desc: gh exited $rc: $(tr '\n' ' ' <"$WORK/stderr.txt")"
		return 1
	fi

	if [ "$out" = "$expected" ]; then
		pass "$desc"
	else
		fail "$desc: expected [$expected], got [$out]"
	fi
}

# expect_gh_failure <description> <gh args...>
expect_gh_failure() {
	local desc=$1
	shift

	local out rc
	out=$(run_gh "$@" 2>&1)
	rc=$?

	if [ "$rc" -ne 0 ]; then
		pass "$desc"
	else
		fail "$desc: gh unexpectedly succeeded with [$out]"
	fi
}

# expect_gateway_request <description> <method> <path>
expect_gateway_request() {
	local pattern="\"method\":\"$2\",\"path\":\"$3\",\"host\":\"$GATEWAY_HOST\",\"auth_header\":true"
	if grep -qF "$pattern" "$LOG" 2>/dev/null; then
		pass "$1"
	else
		fail "$1: no authenticated $2 $3 recorded for $GATEWAY_HOST"
	fi
}

# expect_gateway_request_matching <description> <extended regexp>
expect_gateway_request_matching() {
	if grep -qE "$2" "$LOG" 2>/dev/null; then
		pass "$1"
	else
		fail "$1: no gateway record matching $2"
	fi
}

expect_gateway_silent() {
	local count
	count=$(wc -l <"$LOG" 2>/dev/null | tr -d ' ')
	if [ "${count:-0}" = "0" ]; then
		pass "$1"
	else
		fail "$1: gateway recorded $count requests"
	fi
}

reset_log() {
	: >"$LOG"
}

# --- phase 1: routed ---------------------------------------------------------

heading "Phase 1: api_host set, api.github.com blackholed"
PHASE=1
write_config yes
blackhole_on
reset_log

expect_gh "gh api user returns the authenticated login" "$EXPECTED_LOGIN" \
	api user --jq .login
expect_gh "gh api repos/cli/cli returns real repository data" "cli/cli" \
	api repos/cli/cli --jq .full_name
expect_gh "gh api graphql returns the authenticated login" "$EXPECTED_LOGIN" \
	api graphql -f query='query{viewer{login}}' --jq .data.viewer.login
expect_gh "gh repo view returns real repository data" "cli/cli" \
	repo view cli/cli --json nameWithOwner --jq .nameWithOwner

labels=$(run_gh api --paginate 'repos/cli/cli/labels?per_page=50' --jq '.[].name' 2>"$WORK/stderr.txt")
label_count=$(printf '%s\n' "$labels" | grep -c . )
if [ "$label_count" -gt 50 ]; then
	pass "gh api --paginate followed the rewritten Link header ($label_count labels)"
else
	fail "gh api --paginate did not get past the first page (got $label_count labels): $(tr '\n' ' ' <"$WORK/stderr.txt")"
fi

expect_gateway_request "gateway recorded the authenticated REST request for /user" GET /user
expect_gateway_request "gateway recorded the authenticated REST request for the repository" GET /repos/cli/cli
expect_gateway_request "gateway recorded the authenticated GraphQL requests" POST /graphql
expect_gateway_request_matching "gateway recorded the second page of labels" \
	'"path":"[^"]*page=2[^"]*","host":"'"$GATEWAY_HOST"'"'
# Requests to a gateway-provided URL still have to carry the token, or
# paginating anything private would break. go-gh permits authorization for the
# configured API host for exactly this reason.
expect_gateway_request_matching "second page request carried the token" \
	'"path":"[^"]*page=2[^"]*","host":"'"$GATEWAY_HOST"'","auth_header":true'

heading "Gateway log for phase 1"
if [ -s "$LOG" ]; then
	cat "$LOG"
else
	printf '(empty)\n'
fi

# --- phase 2: control --------------------------------------------------------

heading "Phase 2: no api_host, no blackhole"
PHASE=2
write_config no
blackhole_off
reset_log

expect_gh "gh api user still works without an override" "$EXPECTED_LOGIN" \
	api user --jq .login
expect_gh "gh api repos/cli/cli still works without an override" "cli/cli" \
	api repos/cli/cli --jq .full_name
expect_gh "gh api graphql still works without an override" "$EXPECTED_LOGIN" \
	api graphql -f query='query{viewer{login}}' --jq .data.viewer.login
expect_gh "gh repo view still works without an override" "cli/cli" \
	repo view cli/cli --json nameWithOwner --jq .nameWithOwner

expect_gateway_silent "gateway saw no traffic without an override"

# --- phase 3: blackhole sanity ----------------------------------------------

heading "Phase 3: no api_host, api.github.com blackholed"
PHASE=3
blackhole_on
reset_log

expect_gh_failure "gh cannot reach GitHub directly while blackholed" api user

blackhole_off

# --- phase 4 & 5: acceptance subset ------------------------------------------
# Fine-grained PATs have mutually exclusive resource owners: Account permissions
# (such as Gists) can only be used when the resource owner is the current user,
# while Organization permissions can only be used when the resource owner is an
# organization. A single fine-grained PAT cannot cover both the org-scoped
# scripts and the gist script. Phase 4 therefore runs the eleven org-scoped
# scripts under GH_APIHOST_ORG_TOKEN (a PAT owned by gh-acceptance-testing),
# and phase 5 runs the one gist script under the developer's OAuth token.
if [ "${GH_APIHOST_ACCEPTANCE:-no}" = "yes" ]; then
	[ -n "$ORG_TOKEN" ] || die "GH_APIHOST_ORG_TOKEN is required for the acceptance run"

	# run_subset <group> <script> <token> runs exactly one acceptance
	# script. Scripts are run one at a time rather than batched per test
	# group because a group bundles scripts that fail for unrelated
	# reasons, which would hide a script turning green on its own.
	run_subset() {
		local group="$1" script="$2" tok="$3"
		local logfile rc
		logfile="$WORK/subset-${group}-${script%.txtar}.log"
		# Tee to a file so we can inspect output for "no tests to run" while
		# still streaming it live to the terminal. pipefail would make the
		# pipeline exit with tee's status; PIPESTATUS[0] extracts go test's
		# own exit code regardless.
		GH_ACCEPTANCE_HOST=github.com \
		GH_ACCEPTANCE_ORG="$GH_APIHOST_ORG" \
		GH_ACCEPTANCE_TOKEN="$tok" \
		GH_ACCEPTANCE_USER="$EXPECTED_LOGIN" \
		GH_ACCEPTANCE_API_HOST="$GATEWAY_HOST" \
		GH_ACCEPTANCE_GROUP="$group" \
		GH_ACCEPTANCE_SCRIPT="$script" \
		SSL_CERT_FILE="$BUNDLE" \
		go test -tags=acceptance -count=1 -parallel=1 -timeout=45m \
			-run '^TestAcceptance$' ./acceptance 2>&1 | tee "$logfile"
		rc=${PIPESTATUS[0]}
		if [ "$rc" -ne 0 ]; then
			SUBSET_REDS=$((SUBSET_REDS + 1))
			SUBSET_RED_NAMES="${SUBSET_RED_NAMES:+${SUBSET_RED_NAMES} }${script}"
			record "$script" FAIL
		elif grep -qE '^\s*(ok|FAIL)\s+\S+\s+[0-9.]+s \[no tests to run\]' "$logfile"; then
			SUBSET_NOMATCHES=$((SUBSET_NOMATCHES + 1))
			SUBSET_NOMATCH_NAMES="${SUBSET_NOMATCH_NAMES:+${SUBSET_NOMATCH_NAMES} }${script}"
			record "$script" NOMATCH
		else
			record "$script" PASS
		fi
	}

	blackhole_on

	heading "Phase 4: org-scoped acceptance subset through the gateway"
	PHASE=4
	write_config yes "$ORG_TOKEN"

	run_subset api       basic-rest.txtar                        "$ORG_TOKEN"
	run_subset api       basic-graphql.txtar                     "$ORG_TOKEN"
	run_subset release   release-upload-download.txtar           "$ORG_TOKEN"
	run_subset repo      repo-delete.txtar                       "$ORG_TOKEN"
	run_subset repo      repo-list-rename.txtar                  "$ORG_TOKEN"
	run_subset repo      repo-read-file.txtar                    "$ORG_TOKEN"
	run_subset repo      repo-rename-transfer-ownership.txtar    "$ORG_TOKEN"
	run_subset workflow  run-download.txtar                      "$ORG_TOKEN"
	run_subset extension extension.txtar                         "$ORG_TOKEN"
	run_subset search    search-issues.txtar                     "$ORG_TOKEN"
	run_subset auth      auth-status.txtar                       "$ORG_TOKEN"

	heading "Phase 5: user-scoped acceptance subset through the gateway"
	PHASE=5
	write_config yes "$TOKEN"

	run_subset gist gist-create-view-delete.txtar "$TOKEN"

	blackhole_off
fi

# --- summary -----------------------------------------------------------------

heading "Results"
# Named results, phase by phase. This table is what gets pasted into
# docs/api-host-test-harness.md, so it is meant to be read and diffed directly.
printf 'PHASE  RESULT   NAME\n'
while IFS=$'\t' read -r phase status name; do
	printf '%-6s %-8s %s\n' "$phase" "$status" "$name"
done <"$RESULTS"

heading "Summary"
# Phases 4 and 5 are expected to be partly red during the migration period,
# so subset failures are reported but do not drive the exit code. Only the
# phase 1-3 assertion count does that.
if [ "$FAILURES" -eq 0 ] && [ "$SUBSET_REDS" -eq 0 ] && [ "$SUBSET_NOMATCHES" -eq 0 ]; then
	printf 'all assertions passed\n'
	exit 0
fi

if [ "$FAILURES" -gt 0 ]; then
	printf '%d assertion(s) failed\n' "$FAILURES"
fi

if [ "$SUBSET_REDS" -gt 0 ]; then
	printf '%d subset script(s) red: %s\n' "$SUBSET_REDS" "$SUBSET_RED_NAMES"
fi

if [ "$SUBSET_NOMATCHES" -gt 0 ]; then
	printf '%d subset script(s) matched no tests (script may not exist at this revision): %s\n' "$SUBSET_NOMATCHES" "$SUBSET_NOMATCH_NAMES"
fi

if [ "$FAILURES" -gt 0 ]; then
	exit 1
fi

exit 0
