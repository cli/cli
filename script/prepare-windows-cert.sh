#!/bin/bash
set -e

GITHUB_CERT_PASSWORD=$1
DESKTOP_CERT_TOKEN=$2

if [[ -z "$GITHUB_CERT_PASSWORD" || -z "$DESKTOP_CERT_TOKEN" ]]; then
  echo "skipping windows signing prep; cert password or token not found"
  exit 0
fi

curl \
  -H "Authorization: token $DESKTOP_CERT_TOKEN" \
  -H "Accept: application/vnd.github.v3.raw" \
  --output windows-certificate.pfx \
  https://api.github.com/repos/desktop/desktop-secrets/contents/windows-certificate.pfx

openssl pkcs12 -in windows-certificate.pfx -nocerts -nodes -out private-key.pem  -passin pass:${GITHUB_CERT_PASSWORD}
openssl pkcs12 -in windows-certificate.pfx -nokeys -nodes -out certificate.pem -passin pass:${GITHUB_CERT_PASSWORD}
