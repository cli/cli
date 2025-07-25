#!/usr/bin/env bash
#
# Build Boulder and produce a .deb and a .tar.gz.
#
# This script expects to run on Ubuntu, as configured on GitHub Actions runners
# (with curl, make, and git installed).
#
# -e Stops execution in the instance of a command or pipeline error.
# -u Treat unset variables as an error and exit immediately.
set -eu

ARCH="$(uname -m)"
if [ "${ARCH}" != "x86_64" && "${ARCH}" != "amd64" ]; then
  echo "Expected ARCH=x86_64 or amd64, got ${ARCH}"
  exit 1
fi

# Download and unpack our production go version. Ensure that $GO_VERSION is
# already set in the environment (e.g. by the github actions release workflow).
$(dirname -- "${0}")/fetch-and-verify-go.sh "${GO_VERSION}"
sudo tar -C /usr/local -xzf go.tar.gz
export PATH=/usr/local/go/bin:$PATH

#
# Build
#
LDFLAGS="-X \"github.com/letsencrypt/boulder/core.BuildID=${COMMIT_ID}\" -X \"github.com/letsencrypt/boulder/core.BuildTime=$(date -u)\" -X \"github.com/letsencrypt/boulder/core.BuildHost=$(whoami)@$(hostname)\""
GOBIN=$PWD/bin/ GO111MODULE=on go install -mod=vendor -buildvcs=false -ldflags "${LDFLAGS}" ./...

# Set $VERSION to be a simulacrum of what is set in other build environments.
VERSION="${GO_VERSION}.$(date +%s)"

BOULDER="${PWD}"
BUILD="$(mktemp -d)"
TARGET="${BUILD}/opt/boulder"

mkdir -p "${TARGET}/bin"
for NAME in admin boulder ceremony ct-test-srv pardot-test-srv chall-test-srv ; do
  cp -a "bin/${NAME}" "${TARGET}/bin/"
done

mkdir -p "${TARGET}/test"
cp -a "${BOULDER}/test/config/" "${TARGET}/test/config/"

mkdir -p "${TARGET}/sa"
cp -a "${BOULDER}/sa/db/" "${TARGET}/sa/db/"

cp -a "${BOULDER}/data/" "${TARGET}/data/"

mkdir "${BUILD}/DEBIAN"
cat > "${BUILD}/DEBIAN/control" <<-EOF
Package: boulder
Version: 1:${VERSION}
License: Mozilla Public License v2.0
Vendor: ISRG
Architecture: amd64
Maintainer: Community
Section: default
Priority: extra
Homepage: https://github.com/letsencrypt/boulder
Description: Boulder is an ACME-compatible X.509 Certificate Authority
EOF

dpkg-deb -Zgzip -b "${BUILD}" "./boulder-${VERSION}-${COMMIT_ID}.x86_64.deb"
tar -C "${TARGET}" -cpzf "./boulder-${VERSION}-${COMMIT_ID}.amd64.tar.gz" .
