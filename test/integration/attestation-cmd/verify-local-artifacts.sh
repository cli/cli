#!/usr/bin/env bash
set -euo pipefail

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

ghBuildPath="$rootDir/bin/gh"

sigstore02packageFile="$rootDir/pkg/cmd/attestation/test/data/sigstore-js-2.2.0.tgz"
sigstore03packageFile="$rootDir/pkg/cmd/attestation/test/data/sigstore-js-3.0.0.tgz"
sigstoreBundle02="$rootDir/pkg/cmd/attestation/test/data/sigstore.js-2.2.0.bundle.json"
sigstoreBundle03="$rootDir/pkg/cmd/attestation/test/data/sigstore.js-3.0.0.bundle.json"

# Verify the v0.2.0 sigstore bundle
if ! $ghBuildPath attestation verify "$sigstore02packageFile" -b "$sigstoreBundle02" --digest-alg=sha512 --owner=sigstore; then
    echo "Failed to verify package with a Sigstore v0.2.0 bundle"
    # cleanup test data
    rm "$packageFile" "$attestationFile"
    exit 1
fi

# Verify the v0.3.0 sigstore bundle
if ! $ghBuildPath attestation verify "$sigstore03packageFile" -b "$sigstoreBundle03" --digest-alg=sha512 --owner=sigstore; then
    echo "Failed to verify package with a Sigstore v0.3.0 bundle"
    # cleanup test data
    rm "$packageFile" "$attestationFile"
    exit 1
fi
