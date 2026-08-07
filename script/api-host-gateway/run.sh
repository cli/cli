#!/usr/bin/env bash
# Entry point for the api_host black box test. Runs the test inside a Linux
# container because it needs root to bind port 443 and needs Go to honour
# SSL_CERT_FILE, which it does not do on macOS. See README.md.

set -euo pipefail

IMAGE=${GH_APIHOST_IMAGE:-golang:1.26}
MODULE_CACHE_VOLUME=gh-api-host-gateway-gomodcache
BUILD_CACHE_VOLUME=gh-api-host-gateway-gobuildcache

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

command -v docker >/dev/null 2>&1 || {
	printf 'error: docker is required\n' >&2
	exit 1
}

# The container builds gh from the working tree, not from a commit, so a dirty
# tree silently tests something other than what you think you are testing. That
# matters here more than usual: a result is only evidence if you can say which
# revision produced it.
revision=$(git -C "$repo_root" rev-parse --short HEAD 2>/dev/null || printf 'unknown')
if [ -n "$(git -C "$repo_root" status --porcelain 2>/dev/null)" ]; then
	if [ "${GH_APIHOST_ALLOW_DIRTY:-no}" != "yes" ]; then
		printf 'error: the working tree at %s is dirty\n' "$repo_root" >&2
		printf '\n' >&2
		printf 'gh is built from the working tree, so the result would not\n' >&2
		printf 'describe %s or any other revision. Check out a clean tree,\n' "$revision" >&2
		printf 'or set GH_APIHOST_ALLOW_DIRTY=yes to run anyway.\n' >&2
		exit 1
	fi
	revision="$revision-dirty"
fi
printf 'Testing gh at %s\n\n' "$revision"

token=${GH_TOKEN:-}
if [ -z "$token" ]; then
	token=$(gh auth token --hostname github.com) || {
		printf 'error: set GH_TOKEN or run gh auth login for github.com\n' >&2
		exit 1
	}
fi

org_token=
if [ "${GH_APIHOST_ACCEPTANCE:-no}" = "yes" ]; then
	if [ -n "${GH_APIHOST_ORG_TOKEN:-}" ]; then
		org_token="$GH_APIHOST_ORG_TOKEN"
	elif [ -t 0 ]; then
		printf 'The org run requires a fine-grained PAT with resource owner gh-acceptance-testing.\n' >&2
		printf 'The token must be scoped to all repositories because the scripts create\n' >&2
		printf 'repositories with random names that cannot be listed ahead of time.\n' >&2
		printf '\nCreate one at:\n' >&2
		printf '  %s\n\n' \
			'https://github.com/settings/personal-access-tokens/new?name=gh-acceptance-api-host&description=Acceptance+subset+for+the+api_host+gateway+loop&target_name=gh-acceptance-testing&expires_in=7&administration=write&contents=write&actions=write&workflows=write&issues=write' >&2
		printf 'Paste the token: ' >&2
		read -rs org_token </dev/tty || true
		printf '\n' >&2
	else
		printf 'error: stdin is not a terminal; set GH_APIHOST_ORG_TOKEN instead\n' >&2
		exit 1
	fi
	if [ -z "$org_token" ]; then
		printf 'error: org token is required for the acceptance run\n' >&2
		exit 1
	fi
	# The token is written into a YAML file unquoted, so a stray control
	# character makes that file unparseable and every acceptance script fails
	# for a reason that has nothing to do with routing. Arrow keys at the
	# prompt are the easy way to do this: read -s happily accepts the escape
	# sequence as part of the token.
	case $org_token in
	*[![:print:]]*)
		printf 'error: the org token contains non-printable characters\n' >&2
		printf '\n' >&2
		printf 'Arrow keys and other control characters are captured verbatim at\n' >&2
		printf 'the prompt. Paste the token without editing it, or set\n' >&2
		printf 'GH_APIHOST_ORG_TOKEN instead.\n' >&2
		exit 1
		;;
	esac
fi

# A go.mod replace pointing at a directory on this machine is invisible inside
# the container unless we mount it, so collect any such directories and bind
# them at the same absolute path the replace names. There are none once the
# temporary go-gh replace is reverted, and none at older revisions.
replace_mounts=()
while read -r target; do
	case "$target" in
	/*) ;;
	*) continue ;;
	esac
	[ -d "$target" ] || {
		printf 'error: go.mod replaces a module with %s, which does not exist\n' "$target" >&2
		exit 1
	}
	replace_mounts+=(-v "$target:$target:ro")
done < <(awk '/^replace|=>/ { for (i = 1; i < NF; i++) if ($i == "=>") print $(i + 1) }' "$repo_root/go.mod")

docker volume create "$MODULE_CACHE_VOLUME" >/dev/null
docker volume create "$BUILD_CACHE_VOLUME" >/dev/null

exec docker run --rm -t \
	-v "$repo_root:/src:ro" \
	${replace_mounts[@]+"${replace_mounts[@]}"} \
	-v "$MODULE_CACHE_VOLUME:/go/pkg/mod" \
	-v "$BUILD_CACHE_VOLUME:/root/.cache/go-build" \
	-e GH_APIHOST_TOKEN="$token" \
	-e GH_APIHOST_ORG_TOKEN="$org_token" \
	-e GH_APIHOST_EXPECTED_LOGIN="${GH_APIHOST_EXPECTED_LOGIN:-}" \
	-e GH_APIHOST_ORG="${GH_APIHOST_ORG:-}" \
	-e GH_APIHOST_ACCEPTANCE="${GH_APIHOST_ACCEPTANCE:-no}" \
	-w /src \
	"$IMAGE" \
	/src/script/api-host-gateway/test.sh
