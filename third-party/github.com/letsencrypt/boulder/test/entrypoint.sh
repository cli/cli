#!/usr/bin/env bash

set -e -u

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Start rsyslog. Note: Sometimes for unknown reasons /var/run/rsyslogd.pid is
# already present, which prevents the whole container from starting. We remove
# it just in case it's there.
rm -f /var/run/rsyslogd.pid
service rsyslog start

# make sure we can reach the mysqldb.
./test/wait-for-it.sh boulder-mysql 3306

# make sure we can reach the proxysql.
./test/wait-for-it.sh bproxysql 6032

# create the database
MYSQL_CONTAINER=1 $DIR/create_db.sh

if [[ $# -eq 0 ]]; then
    exec python3 ./start.py
fi

exec "$@"
