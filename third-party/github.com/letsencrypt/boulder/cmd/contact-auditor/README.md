# Contact-Auditor

Audits subscriber registrations for e-mail addresses that
`notify-mailer` is currently configured to skip.

# Usage:

```shell
  -config string
      File containing a JSON config.
  -to-file
      Write the audit results to a file.
  -to-stdout
      Print the audit results to stdout.
```

## Results format:

```
<id>    <createdAt>    <problem type>    "<contact contents or entry>"    "<error msg>"
```

## Example output:

### Successful run with no violations encountered and `--to-file`:

```
I004823 contact-auditor nfWK_gM Running contact-auditor
I004823 contact-auditor qJ_zsQ4 Beginning database query
I004823 contact-auditor je7V9QM Query completed successfully
I004823 contact-auditor 7LzGvQI Audit finished successfully
I004823 contact-auditor 5Pbk_QM Audit results were written to: audit-2006-01-02T15:04.tsv
```

### Contact contains entries that violate policy and `--to-stdout`:

```
I004823 contact-auditor nfWK_gM Running contact-auditor
I004823 contact-auditor qJ_zsQ4 Beginning database query
I004823 contact-auditor je7V9QM Query completed successfully
1    2006-01-02 15:04:05    validation    "<contact entry>"    "<error msg>"
...
I004823 contact-auditor 2fv7-QY Audit finished successfully
```

### Contact is not valid JSON and `--to-stdout`:

```
I004823 contact-auditor nfWK_gM Running contact-auditor
I004823 contact-auditor qJ_zsQ4 Beginning database query
I004823 contact-auditor je7V9QM Query completed successfully
3    2006-01-02 15:04:05    unmarshal    "<contact contents>"    "<error msg>"
...
I004823 contact-auditor 2fv7-QY Audit finished successfully
```

### Audit incomplete, query ended prematurely:

```
I004823 contact-auditor nfWK_gM Running contact-auditor
I004823 contact-auditor qJ_zsQ4 Beginning database query
...
E004823 contact-auditor 8LmTgww [AUDIT] Audit was interrupted, results may be incomplete: <error msg>
exit status 1
```

# Configuration file:
The path to a database config file like the one below must be provided
following the `-config` flag.

```json
{
    "contactAuditor": {
      "db": {
        "dbConnectFile": <string>,
        "maxOpenConns": <int>,
        "maxIdleConns": <int>,
	      "connMaxLifetime": <int>,
	      "connMaxIdleTime": <int>
      }
    }
  }
  
```
