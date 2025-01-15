#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <matrix-os>"
  exit 1
fi

os=$1

# Get the root directory of the repository
rootDir="$(git rev-parse --show-toplevel)"

verify_test_dir="$rootDir/test/integration/attestation-cmd/verify"
echo "Running all \"gh attestation verify\" tests"
for script in "$verify_test_dir"/*.sh; do
  if [ -f "$script" ]; then
    echo "Running $script..."
    bash "$script"
  fi
done

download_test_dir="$rootDir/test/integration/attestation-cmd/download"
echo "Running all \"gh attestation download\" tests"
for script in "$download_test_dir"/*.sh; do
  if [ -f "$script" ]; then
    echo "Running $script..."
    bash "$script" "$os"
  fi
done
