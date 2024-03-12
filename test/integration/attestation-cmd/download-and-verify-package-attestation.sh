#!/usr/bin/env bash
set -euo pipefail

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

ghBuildPath="$rootDir/bin/gh"

# Compute the package and attestation URLs
labRatPackageName="sigstore"
latestPackageVersion=$(npm -s info $labRatPackageName dist-tags.latest | tr -d '\n')
packageFile="$labRatPackageName-$latestPackageVersion.tgz"
packageURL="https://registry.npmjs.org/$labRatPackageName/-/$packageFile"
attestationFile="$labRatPackageName-$latestPackageVersion.json"
attestationURL="https://registry.npmjs.org/-/npm/v1/attestations/$labRatPackageName@$latestPackageVersion"

echo "Testing with package $packageFile and attestation $attestationFile"

curl -s "$packageURL" -o "$packageFile"
curl -s "$attestationURL" | jq '.attestations[1].bundle' > "$attestationFile"

# Verify the package with the --owner flag
if ! $ghBuildPath attestation verify "$packageFile" -b "$attestationFile" --digest-alg=sha512 --owner=sigstore; then
    # cleanup test data
    echo "Failed to verify package with --owner flag"
    rm "$packageFile" "$attestationFile"
    exit 1
fi

if ! $ghBuildPath attestation verify "$packageFile" -b "$attestationFile" --digest-alg=sha512 --repo=sigstore/sigstore-js; then
    # cleanup test data
    echo "Failed to verify package with --repo flag"
    rm "$packageFile" "$attestationFile"
    exit 1
fi

# cleanup test data
rm "$packageFile" "$attestationFile"
