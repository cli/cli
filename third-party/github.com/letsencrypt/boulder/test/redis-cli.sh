#!/usr/bin/env bash

set -feuo pipefail

ARGS="-p 4218 \
    --tls \
    --cert /test/certs/ipki/redis/cert.pem \
    --key /test/certs/ipki/redis/key.pem \
    --cacert /test/certs/ipki/minica.pem \
    --user admin-user \
    --pass 435e9c4225f08813ef3af7c725f0d30d263b9cd3"

exec docker compose exec bredis_1 redis-cli $ARGS "${@}"
