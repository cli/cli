# Logging

Boulder can log to stdout/stderr, syslog, or both. Boulder components
generally have a `syslog` portion of their JSON config that indicates the
maximum level of log that should be sent to a given destination. For instance,
in `test/config/wfe2.json`:

```
  "syslog": {
    "stdoutlevel": 4,
    "sysloglevel": 6
  },
```

This indicates that logs of level 4 or below (error and warning) should be
emitted to stdout/stderr, and logs of level 6 or below (error, warning, notice, and
info) should be emitted to syslog, using the local Unix socket method. The
highest meaningful value is 7, which enables debug logging.

The stdout/stderr logger uses ANSI escape codes to color warnings as yellow
and errors as red, if stdout is detected to be a terminal.

The default value for these fields is 6 (INFO) for syslogLevel and 0 (no logs)
for stdoutLevel. To turn off syslog logging entirely, set syslogLevel to -1.

In Boulder's development environment, we enable stdout logging because that
makes it easier to see what's going on quickly. In production, we disable stdout
logging because it would duplicate the syslog logging. We preferred the syslog
logging because it provides things like severity level in a consistent way with
other components. But we may move to stdout/stderr logging to make it easier to
containerize Boulder.

Boulder has a number of adapters to take other packages' log APIs and send them
to syslog as expected. For instance, we provide a custom logger for mysql, grpc,
and prometheus that forwards to syslog. This is configured in StatsAndLogging in
cmd/shell.go.

There are some cases where we output to stdout regardless of the JSON config
settings:

 - Panics are always emitted to stdout
 - Packages that Boulder relies on may occasionally emit to stdout (though this
   is generally not ideal and we try to get it changed).

Typically these output lines will be collected by systemd and forwarded to
syslog.

## Verification

We attach a simple checksum to each log line. This is not a cryptographically
secure hash, but is intended to let us catch corruption in the log system. This
is a short chunk of base64 encoded data near the beginning of the log line. It
is consumed by cmd/log-validator.
