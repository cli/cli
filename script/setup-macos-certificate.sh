#!/bin/bash
set -eu

CERT_URL="https://api.github.com/repos/desktop/desktop-secrets/contents/developer-id-cert.p12"
CERT_FILE="$(basename "$CERT_URL")"
KEY_CHAIN="mac-build.keychain"
keychain_password="bumblebee"

curl -fsSL -H "Authorization: token ${DESKTOP_CERT_TOKEN?}" \
  -H 'Accept: application/vnd.github.v3.raw' -O "$CERT_URL"

trap "rm -f '$CERT_FILE'" EXIT

security create-keychain -p "$keychain_password" "$KEY_CHAIN"
security import "$CERT_FILE" -k "$KEY_CHAIN" -P "${CERT_PASSWORD?}" -T /usr/bin/codesign

security list-keychain -s "$KEY_CHAIN"
security unlock-keychain -p "$keychain_password" "$KEY_CHAIN"
security set-keychain-settings -t 3600 -u "$KEY_CHAIN"

security default-keychain -s "$KEY_CHAIN"

# set key partition list to avoid UI permission popup that causes hanging at CI
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$keychain_password" "$KEY_CHAIN"

echo "::set-output name=keychain::$KEY_CHAIN"
