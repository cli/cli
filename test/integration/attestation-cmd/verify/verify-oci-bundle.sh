#!/usr/bin/env bash
set -euo pipefail

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

ghBuildPath="$rootDir/bin/gh"

# Verify an OCI artifact with bundles stored on the GHCR OCI registry
echo "Testing with OCI image ghcr.io/github/artifact-attestations-helm-charts/policy-controller:v0.10.0-github9 with the --bundle-from-oci flag"
if ! $ghBuildPath attestation verify oci://ghcr.io/github/artifact-attestations-helm-charts/policy-controller:v0.10.0-github9 --owner=github --bundle-from-oci; then
    echo "Failed to verify oci://ghcr.io/github/artifact-attestations-helm-charts/policy-controller:v0.10.0-github9 with bundles from the GHCR OCI registry"
    exit 1
fi
