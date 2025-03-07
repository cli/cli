#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <matrix-os>"
  exit 1
fi

os=$1

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

ghBuildPath="$rootDir/bin/gh"

artifactPath="$rootDir/pkg/cmd/attestation/test/data/gh_2.60.1_windows_arm64.zip"

# Download attestations for the package
if ! $ghBuildPath attestation download "$artifactPath" --owner=cli; then
    # cleanup test data
    echo "Failed to download attestations"
    exit 1
fi

digest="5ddb1d4d013a44c2e5df027867c0d4161383eb7c16e569a86384af52bfe09a65"
attestation_filename="sha256:$digest.jsonl"
if [ "$os" == "windows-latest" ]; then
  echo "Running the test on Windows."
  echo "Build the expected filename accordingly"
  attestation_filename="sha256-$digest.jsonl"
fi

if [ ! -f "$attestation_filename" ]; then
  echo "Expected attestation file $attestation_filename not found"
  exit 1
fi

if [ ! -s "$attestation_filename" ]; then
  echo "Attestation file $attestation_filename is empty"
  rm "$attestation_filename"
  exit 1
fi

cat "$attestation_filename"

# Clean up the downloaded attestation file
rm "$attestation_filename"
