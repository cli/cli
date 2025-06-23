#!/bin/bash

set -e -u

wait_tcp_port() {
    local host="${1}" port="${2}"

    # see http://tldp.org/LDP/abs/html/devref1.html for description of this syntax.
    local max_tries="40"
    for n in `seq 1 "${max_tries}"` ; do
      if { exec 6<>/dev/tcp/"${host}"/"${port}" ; } 2>/dev/null ; then
        break
      else
        echo "$(date) - still trying to connect to ${host}:${port}"
        sleep 1
      fi
      if [ "${n}" -eq "${max_tries}" ]; then
        echo "unable to connect"
        exit 1
      fi
    done
    exec 6>&-
    echo "Connected to ${host}:${port}"
}

wait_tcp_port "${1}" "${2}"
shift 2
exec "$@"
