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

echo "Testing with package $packageFile"

curl -s "$packageURL" -o "$packageFile"

# Verify the package with the --owner flag
if ! $ghBuildPath attestation verify "$packageFile" --digest-alg=sha512 --owner=sigstore; then
    # cleanup test data
    echo "Failed to verify package with --owner flag"
    rm "$packageFile"
    exit 1
fi

if ! $ghBuildPath attestation verify "$packageFile" --digest-alg=sha512 --repo=sigstore/sigstore-js; then
    # cleanup test data
    echo "Failed to verify package with --repo flag"
    rm "$packageFile"
    exit 1
fi

# cleanup test data
rm "$packageFile"
