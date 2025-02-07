#!/usr/bin/env bash
set -euo pipefail

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

ghBuildPath="$rootDir/bin/gh"

artifactPath="$rootDir/pkg/cmd/attestation/test/data/sigstore-js-2.1.0.tgz"
bundlePath="$rootDir/pkg/cmd/attestation/test/data/sigstore-js-2.1.0_with_2_bundles.jsonl"

# Download a custom trusted root for verification
if ! $ghBuildPath attestation trusted-root > trusted_root.jsonl; then
    # cleanup test data
    echo "Failed to download trusted root"
    exit 1
fi

if ! $ghBuildPath attestation verify "$artifactPath" -b "$bundlePath" --digest-alg=sha512 --owner=sigstore --custom-trusted-root trusted_root.jsonl; then
    echo "Failed to verify package with a Sigstore v0.2.0 bundle"
    exit 1
fi
