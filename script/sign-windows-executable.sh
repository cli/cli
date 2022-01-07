#!/bin/bash
set -e

EXECUTABLE_PATH=$1

curl \
  -H "Authorization: token $DESKTOP_CERT_TOKEN" \
  -H "Accept: application/vnd.github.v3.raw" \
  --output windows-certificate.pfx \
  --silent \
  https://api.github.com/repos/desktop/desktop-secrets/contents/windows-certificate.pfx

PROGRAM_NAME="GitHub CLI"

# Convert private key to the expected format
openssl pkcs12 -in windows-certificate.pfx -nocerts -nodes -out private-key.pem  -passin pass:${GITHUB_CERT_PASSWORD}
openssl rsa -in private-key.pem -outform PVK -pvk-none -out private-key.pvk 2>/dev/null # Always writes to STDERR

# Convert certificate chain into the expected format
openssl pkcs12 -in windows-certificate.pfx -nokeys -nodes -out certificate.pem -passin pass:${GITHUB_CERT_PASSWORD}
openssl crl2pkcs7 -nocrl -certfile certificate.pem -outform DER -out certificate.spc

signcode \
  -spc certificate.spc \
  -v private-key.pvk \
  -n $PROGRAM_NAME \
  -t http://timestamp.digicert.com \
  -a sha256 \
$EXECUTABLE_PATH \
1> /dev/null # STDOUT a little bit chatty here, with multiple lines
