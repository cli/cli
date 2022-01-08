#!/bin/bash
set -e

EXECUTABLE_PATH=$1

curl \
  -H "Authorization: token $DESKTOP_CERT_TOKEN" \
  -H "Accept: application/vnd.github.v3.raw" \
  --output windows-certificate.pfx \
  https://api.github.com/repos/desktop/desktop-secrets/contents/windows-certificate.pfx

openssl pkcs12 -in windows-certificate.pfx -nocerts -nodes -out private-key.pem  -passin pass:${GITHUB_CERT_PASSWORD}
openssl pkcs12 -in windows-certificate.pfx -nokeys -nodes -out certificate.pem -passin pass:${GITHUB_CERT_PASSWORD}

osslsigncode sign \
  -certs certificate.pem \
  -key private-key.pem \
  -n "GitHub CLI" \
  -t http://timestamp.digicert.com \
  -in $EXECUTABLE_PATH \
  -out gh_signed.exe

# Oddly, there can be a delay before the file is *actually* available - wait for it
while [ ! -f gh_signed.exe ]; do sleep 1; done;

mv gh_signed.exe $EXECUTABLE_PATH
