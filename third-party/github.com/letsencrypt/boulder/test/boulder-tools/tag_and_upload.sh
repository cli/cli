#!/bin/bash

set -feuxo pipefail

cd $(dirname $0)

DATESTAMP=$(date +%Y-%m-%d)
DOCKER_REPO="letsencrypt/boulder-tools"

# These versions are only built for platforms that we run in CI.
# When updating these GO_CI_VERSIONS, please also update
# .github/workflows/release.yml,
# .github/workflows/try-release.yml if appropriate,
# and .github/workflows/boulder-ci.yml with the new container tag.
GO_CI_VERSIONS=( "1.24.4" )

echo "Please login to allow push to DockerHub"
docker login

# Usage: build_and_push_image $GO_VERSION
build_and_push_image() {
  GO_VERSION="$1"
  TAG_NAME="${DOCKER_REPO}:go${GO_VERSION}_${DATESTAMP}"
  echo "Building boulder-tools image ${TAG_NAME}"

  # build, tag, and push the image.
  docker buildx build \
    --build-arg "GO_VERSION=${GO_VERSION}" \
    --progress plain \
    --push \
    --tag "${TAG_NAME}" \
    --platform "linux/amd64" \
    .
}

for GO_VERSION in "${GO_CI_VERSIONS[@]}"
do
  build_and_push_image $GO_VERSION
done
