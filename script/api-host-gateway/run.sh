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

token=${GH_TOKEN:-}
if [ -z "$token" ]; then
	token=$(gh auth token --hostname github.com) || {
		printf 'error: set GH_TOKEN or run gh auth login for github.com\n' >&2
		exit 1
	}
fi

docker volume create "$MODULE_CACHE_VOLUME" >/dev/null
docker volume create "$BUILD_CACHE_VOLUME" >/dev/null

exec docker run --rm -t \
	-v "$repo_root:/src:ro" \
	-v "$MODULE_CACHE_VOLUME:/go/pkg/mod" \
	-v "$BUILD_CACHE_VOLUME:/root/.cache/go-build" \
	-e GH_APIHOST_TOKEN="$token" \
	-e GH_APIHOST_EXPECTED_LOGIN="${GH_APIHOST_EXPECTED_LOGIN:-williammartin}" \
	-e GH_APIHOST_ORG="${GH_APIHOST_ORG:-}" \
	-e GH_APIHOST_ACCEPTANCE="${GH_APIHOST_ACCEPTANCE:-no}" \
	-w /src \
	"$IMAGE" \
	/src/script/api-host-gateway/test.sh
