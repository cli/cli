#!/bin/bash
set -e

cd "$(realpath -- $(dirname -- "$0"))"

# Check that `minica` is installed
command -v minica >/dev/null 2>&1 || {
  echo >&2 "No 'minica' command available.";
  echo >&2 "Check your GOPATH and run: 'go install github.com/jsha/minica@latest'.";
  exit 1;
}

ipki() (
  # Minica generates everything in-place, so we need to cd into the subdirectory.
  # This function executes in a subshell, so this cd does not affect the parent
  # script.
  mkdir ipki
  cd ipki

  # Create a generic cert which can be used by our test-only services that
  # aren't sophisticated enough to present a different name. This first
  # invocation also creates the issuer key, so the loops below can run in the
  # background without racing to create it.
  minica -domains localhost --ip-addresses 127.0.0.1

  # Used by challtestsrv to negotiate DoH handshakes. Even though we think of
  # challtestsrv as being external to our infrastructure (because it hosts the
  # DNS records that the tests validate), it *also* takes the place of our
  # recursive resolvers, so the DoH certificate that it presents to the VAs is
  # part of our internal PKI.
  minica -ip-addresses 10.77.77.77,10.88.88.88

  # Presented by the WFE's TLS server, when configured. Normally the WFE lives
  # behind another TLS-terminating server like nginx or apache, so the cert that
  # it presents to that layer is also part of the internal PKI.
  minica -domains "boulder"

  # Presented by the test redis cluster. Contains IP addresses because Boulder
  # components find individual redis servers via SRV records.
  minica -domains redis -ip-addresses 10.77.77.2,10.77.77.3,10.77.77.4,10.77.77.5

  # Used by Boulder gRPC services as both server and client mTLS certificates.
  for SERVICE in admin ocsp-responder consul \
    wfe akamai-purger bad-key-revoker crl-updater crl-storer \
    health-checker rocsp-tool sfe email-exporter; do
    minica -domains "${SERVICE}.boulder" &
  done

  # Same as above, for services that we run multiple copies of.
  for SERVICE in publisher nonce ra ca sa va rva ; do
    minica -domains "${SERVICE}.boulder,${SERVICE}1.boulder,${SERVICE}2.boulder" &
  done

  wait

  # minica sets restrictive directory permissions, but we don't want that
  chmod -R go+rX .
)

webpki() (
  # Because it invokes the ceremony tool, webpki.go expects to be invoked with
  # the root of the boulder repo as the current working directory.
  # This function executes in a subshell, so this cd does not affect the parent
  # script.
  cd ../..
  make build
  mkdir ./test/certs/webpki
  go run ./test/certs/webpki.go
)

if ! [ -d ipki ]; then
  echo "Generating ipki/..."
  ipki
fi

if ! [ -d webpki ]; then
  echo "Generating webpki/..."
  webpki
fi
