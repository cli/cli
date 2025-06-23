# ProxySQL in Boulder

In an effort to keep Boulder's development environment reasonably close to
production we use ProxySQL in our Docker stack to proxy connections to our
MariaDB database.

## Ports

ProxySQL listens on the following ports:
  - `6033` Proxy MySQL Interface
  - `6032` Admin MySQL Interface
  - `6080` Admin Web Interface

## Accessing the Admin MySQL Interface

```bash
mysql -uradmin -pradmin -h 127.0.0.1 --port 6032
```

### MacOS

You will need to bind the port in `docker-compose.yml`, like so:

```yaml
  bproxysql:
    ports:
      - 6032:6032
```

## Accessing the Admin Web Interface

You can access the ProxySQL web UI at https://127.0.0.1:6080. The default
username/ password are `stats`/ `stats`.

### MacOS

You will need to bind the port in `docker-compose.yml`, like so:

```yaml
  bproxysql:
    ports:
      - 6080:6080
```

## Sending queries to a file

To log all queries routed through the ProxySQL query parser, uncomment the
following line in the `mysql_variables` section of `test/proxysql/proxysql.cnf`,
like so:

```ini
# If mysql_query_rules are marked log=1, they will be logged here. If unset,
# no queries are logged.
eventslog_filename="/test/proxysql/events.log"
```

Then set `log = 1;` for `rule_id = 1;` in the `mysql_query_rules` section, like so:

```
{
    rule_id = 1;
    active = 1;
    # Log all queries.
    match_digest = ".";
    # Set log=1 to log all queries to the eventslog_filename under
    # mysql_variables.
    log = 1;
    apply = 0;
},
```

## Sending ProxySQL logs to a file

Replace the `entrypoint:` under `bproxysql` in `docker-compose.yml` with
`/test/proxysql/entrypoint.sh`. This is necessary because if you attempt to run
ProxySQL in the background (by removing the `-f` flag) Docker will simply kill
the container.
