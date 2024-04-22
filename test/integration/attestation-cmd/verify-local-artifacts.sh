#!/usr/bin/env bash
set -euo pipefail

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

ghBuildPath="$rootDir/bin/gh"

sigstore02PackageFile="sigstore-2.2.0.tgz"
sigstore02PackageURL="https://registry.npmjs.org/sigstore/-/$sigstore02PackageFile"
sigstore02AttestationFile="sigstore-2.2.0.json"
sigstore02AttestationURL="https://registry.npmjs.org/-/npm/v1/attestations/sigstore@2.2.0"

curl -s "$sigstore02PackageURL" -o "$sigstore02PackageFile"
curl -s "$sigstore02AttestationURL" | jq '.attestations[1].bundle' > "$sigstore02AttestationFile"

sigstore03PackageFile="sigstore-3.0.0.tgz"
sigstore03PackageURL="https://registry.npmjs.org/sigstore/-/$sigstore03PackageFile"
sigstore03AttestationFile="sigstore-3.0.0.json"
sigstore03AttestationURL="https://registry.npmjs.org/-/npm/v1/attestations/sigstore@3.0.0"

curl -s "$sigstore03PackageURL" -o "$sigstore03PackageFile"
curl -s "$sigstore03AttestationURL" | jq '.attestations[1].bundle' > "$sigstore03AttestationFile"

# Verify the v0.2.0 sigstore bundle
echo "Testing with package $sigstore02PackageFile and attestation $sigstore02AttestationFile"
if ! $ghBuildPath attestation verify "$sigstore02PackageFile" -b "$sigstore02AttestationFile" --digest-alg=sha512 --owner=sigstore; then
    echo "Failed to verify package with a Sigstore v0.2.0 bundle"
    # cleanup test data
    rm "$sigstore02PackageFile" "$sigstore02AttestationFile"
    exit 1
fi

# Verify the v0.3.0 sigstore bundle
echo "Testing with package $sigstore03PackageFile and attestation $sigstore03AttestationFile"
if ! $ghBuildPath attestation verify "$sigstore03PackageFile" -b "$sigstore03AttestationFile" --digest-alg=sha512 --owner=sigstore; then
    echo "Failed to verify package with a Sigstore v0.3.0 bundle"
    # cleanup test data
    rm "$sigstore03PackageFile" "$sigstore03AttestationFile"
    exit 1
fi
