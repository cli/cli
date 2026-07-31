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
EXPECTED_LOGIN=${GH_APIHOST_EXPECTED_LOGIN:-williammartin}
TOKEN=${GH_APIHOST_TOKEN:-}

CONFIG_DIR="$WORK/ghconfig"
LOG="$WORK/gateway.jsonl"
BUNDLE="$WORK/bundle.pem"

FAILURES=0
GATEWAY_PID=

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

pass() {
	printf '  ok   %s\n' "$1"
}

fail() {
	printf '  FAIL %s\n' "$1"
	FAILURES=$((FAILURES + 1))
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

# --- helpers -----------------------------------------------------------------

# write_config yes|no controls whether hosts.yml carries the api_host override.
write_config() {
	{
		printf '%s:\n' "github.com"
		printf '    user: %s\n' "$EXPECTED_LOGIN"
		printf '    oauth_token: %s\n' "$TOKEN"
		printf '    git_protocol: https\n'
		if [ "$1" = "yes" ]; then
			printf '    api_host: %s\n' "$GATEWAY_HOST"
		fi
		printf '    users:\n'
		printf '        %s:\n' "$EXPECTED_LOGIN"
		printf '            oauth_token: %s\n' "$TOKEN"
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

	if [ $rc -ne 0 ]; then
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

	if [ $rc -ne 0 ]; then
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
blackhole_on
reset_log

expect_gh_failure "gh cannot reach GitHub directly while blackholed" api user

blackhole_off

# --- summary -----------------------------------------------------------------

heading "Summary"
if [ "$FAILURES" -eq 0 ]; then
	printf 'all assertions passed\n'
	exit 0
fi

printf '%d assertion(s) failed\n' "$FAILURES"
exit 1
