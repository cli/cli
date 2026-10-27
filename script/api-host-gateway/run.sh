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
	-e GH_APIHOST_EXPECTED_LOGIN="${GH_APIHOST_EXPECTED_LOGIN:-}" \
	-w /src \
	"$IMAGE" \
	/src/script/api-host-gateway/test.sh
