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

curl -s $packageURL -o $packageFile
curl -s $attestationURL | jq '.attestations[1].bundle' > $attestationFile

# Verify the package with the --owner flag
$ghVerifyBuildPath $packageFile -b $attestationFile --digest-alg=sha512 --owner=sigstore
if [ $? -ne 0 ]; then
    # cleanup test data
    rm $packageFile $attestationFile
    exit 1
fi

$ghVerifyBuildPath $packageFile -b $attestationFile --digest-alg=sha512 --repo=sigstore-js
if [ $? -ne 0 ]; then
    # cleanup test data
    rm $packageFile $attestationFile
    exit 1
fi

# cleanup test data
rm $packageFile $attestationFile
