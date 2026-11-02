#!/bin/sh
# Replay with: ./05-attribution.sh --gh /absolute/path/to/gh run-all
exec /usr/bin/env -i PATH=/usr/bin:/bin "REVIEW_ALLOCATION_HOME=${HOME:-}" /bin/sh "${0%/*}/isolated-runner.sh" 05 "$@"
