#!/usr/bin/env bash

exec proxysql -f --idle-threads -c /test/proxysql/proxysql.cnf --initial 2>&1 | tee -a /test/proxysql/proxysql.log
