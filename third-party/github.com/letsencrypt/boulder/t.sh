#!/usr/bin/env bash
#
# Outer wrapper for invoking test.sh inside docker-compose.
#

set -o errexit

if type realpath >/dev/null 2>&1 ; then
  cd "$(realpath -- $(dirname -- "$0"))"
fi

# Generate the test keys and certs necessary for the integration tests.
docker compose run --rm bsetup

exec docker compose run --rm --name boulder_tests boulder ./test.sh "$@"
