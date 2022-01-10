#!/bin/bash
set -e
set -x

EXECUTABLE_PATH=$1

curl \
  -H "Authorization: token $DESKTOP_CERT_TOKEN" \
  -H "Accept: application/vnd.github.v3.raw" \
  --output windows-certificate.pfx \
  https://api.github.com/repos/desktop/desktop-secrets/contents/windows-certificate.pfx

stat windows-certificate.pfx
file windows-certificate.pfx

openssl pkcs12 -in windows-certificate.pfx -nocerts -nodes -out private-key.pem  -passin pass:${GITHUB_CERT_PASSWORD} || echo "no bueno 1"
openssl pkcs12 -in windows-certificate.pfx -nokeys -nodes -out certificate.pem -passin pass:${GITHUB_CERT_PASSWORD} || echo "no bueno 2"

osslsigncode sign \
  -certs certificate.pem \
  -key private-key.pem \
  -n "GitHub CLI" \
  -t http://timestamp.digicert.com \
  -in $EXECUTABLE_PATH \
  -out gh_signed.exe || echo "no bueno 3"

stat gh_signed.exe
file gh_signed.exe

# Oddly, there can be a delay before the file is *actually* available - wait for it
sleep 10
# this was forcing the action to time out which makes debugging harder. 30
# seconds isn't ideal but it's better than 3 minutes.
#while [ ! -f gh_signed.exe ]; do sleep 1; done;

mv gh_signed.exe $EXECUTABLE_PATH
