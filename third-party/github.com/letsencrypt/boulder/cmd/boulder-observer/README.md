# boulder-observer

A modular configuration driven approach to black box monitoring with
Prometheus.

* [boulder-observer](#boulder-observer)
  * [Usage](#usage)
    * [Options](#options)
    * [Starting the boulder-observer
      daemon](#starting-the-boulder-observer-daemon)
  * [Configuration](#configuration)
    * [Root](#root)
      * [Schema](#schema)
      * [Example](#example)
    * [Monitors](#monitors)
      * [Schema](#schema-1)
      * [Example](#example-1)
    * [Probers](#probers)
      * [DNS](#dns)
        * [Schema](#schema-2)
        * [Example](#example-2)
      * [HTTP](#http)
        * [Schema](#schema-3)
        * [Example](#example-3)
      * [CRL](#crl)
        * [Schema](#schema-4)
        * [Example](#example-4)
      * [TLS](#tls)
        * [Schema](#schema-5)
        * [Example](#example-5)
  * [Metrics](#metrics)
    * [Global Metrics](#global-metrics)
      * [obs_monitors](#obs_monitors)
      * [obs_observations](#obs_observations)
    * [CRL Metrics](#crl-metrics)
      * [obs_crl_this_update](#obs_crl_this_update)
      * [obs_crl_next_update](#obs_crl_next_update)
      * [obs_crl_revoked_cert_count](#obs_crl_revoked_cert_count)
    * [TLS Metrics](#tls-metrics)
      * [obs_crl_this_update](#obs_tls_not_after)
      * [obs_crl_next_update](#obs_tls_reason)
  * [Development](#development)
    * [Starting Prometheus locally](#starting-prometheus-locally)
    * [Viewing metrics locally](#viewing-metrics-locally)

## Usage

### Options

```shell
$ ./boulder-observer -help
  -config string
        Path to boulder-observer configuration file (default "config.yml")
```

### Starting the boulder-observer daemon

```shell
$ ./boulder-observer -config test/config-next/observer.yml
I152525 boulder-observer _KzylQI Versions: main=(Unspecified Unspecified) Golang=(go1.16.2) BuildHost=(Unspecified)
I152525 boulder-observer q_D84gk Initializing boulder-observer daemon from config: test/config-next/observer.yml
I152525 boulder-observer 7aq68AQ all monitors passed validation
I152527 boulder-observer yaefiAw kind=[HTTP] success=[true] duration=[0.130097] name=[https://letsencrypt.org-[200]]
I152527 boulder-observer 65CuDAA kind=[HTTP] success=[true] duration=[0.148633] name=[http://letsencrypt.org/foo-[200 404]]
I152530 boulder-observer idi4rwE kind=[DNS] success=[false] duration=[0.000093] name=[[2606:4700:4700::1111]:53-udp-A-google.com-recurse]
I152530 boulder-observer prOnrw8 kind=[DNS] success=[false] duration=[0.000242] name=[[2606:4700:4700::1111]:53-tcp-A-google.com-recurse]
I152530 boulder-observer 6uXugQw kind=[DNS] success=[true] duration=[0.022962] name=[1.1.1.1:53-udp-A-google.com-recurse]
I152530 boulder-observer to7h-wo kind=[DNS] success=[true] duration=[0.029860] name=[owen.ns.cloudflare.com:53-udp-A-letsencrypt.org-no-recurse]
I152530 boulder-observer ovDorAY kind=[DNS] success=[true] duration=[0.033820] name=[owen.ns.cloudflare.com:53-tcp-A-letsencrypt.org-no-recurse]
...
```

## Configuration

Configuration is provided via a YAML file.

### Root

#### Schema

`debugaddr`: The Prometheus scrape port prefixed with a single colon
(e.g. `:8040`).

`buckets`: List of floats representing Prometheus histogram buckets (e.g
`[.001, .002, .005, .01, .02, .05, .1, .2, .5, 1, 2, 5, 10]`)

`syslog`: Map of log levels, see schema below.

- `stdoutlevel`: Log level for stdout, see legend below.
- `sysloglevel`:Log level for stdout, see legend below.

`0`: *EMERG* `1`: *ALERT* `2`: *CRIT* `3`: *ERR* `4`: *WARN* `5`:
*NOTICE* `6`: *INFO* `7`: *DEBUG*

`monitors`: List of monitors, see [monitors](#monitors) for schema.

#### Example

```yaml
debugaddr: :8040
buckets: [.001, .002, .005, .01, .02, .05, .1, .2, .5, 1, 2, 5, 10]
syslog:
  stdoutlevel: 6
  sysloglevel: 6
  -
    ...
```

### Monitors

#### Schema

`period`: Interval between probing attempts (e.g. `1s` `1m` `1h`).

`kind`: Kind of prober to use, see [probers](#probers) for schema.

`settings`: Map of prober settings, see [probers](#probers) for schema.

#### Example

```yaml
monitors:
  - 
    period: 5s
    kind: DNS
    settings:
        ...
```

### Probers

#### DNS

##### Schema

`protocol`: Protocol to use, options are: `udp` or `tcp`.

`server`: Hostname, IPv4 address, or IPv6 address surrounded with
brackets + port of the DNS server to send the query to (e.g.
`example.com:53`, `1.1.1.1:53`, or `[2606:4700:4700::1111]:53`).

`recurse`: Bool indicating if recursive resolution is desired.

`query_name`: Name to query (e.g. `example.com`).

`query_type`: Record type to query, options are: `A`, `AAAA`, `TXT`, or
`CAA`.

##### Example

```yaml
monitors:
  - 
    period: 5s
    kind: DNS
    settings:
      protocol: tcp
      server: [2606:4700:4700::1111]:53
      recurse: false
      query_name: letsencrypt.org
      query_type: A
```

#### HTTP

##### Schema

`url`: Scheme + Hostname to send a request to (e.g.
`https://example.com`).

`rcodes`: List of expected HTTP response codes.

`useragent`: String to set HTTP header User-Agent. If no useragent string
is provided it will default to `letsencrypt/boulder-observer-http-client`.

##### Example

```yaml
monitors:
  - 
    period: 2s
    kind: HTTP
    settings:
      url: http://letsencrypt.org/FOO
      rcodes: [200, 404]
      useragent: letsencrypt/boulder-observer-http-client
```

#### CRL

##### Schema

`url`: Scheme + Hostname to grab the CRL from (e.g. `http://x1.c.lencr.org/`).

##### Example

```yaml
monitors:
  - 
    period: 1h
    kind: CRL
    settings:
      url: http://x1.c.lencr.org/
```

#### TLS

##### Schema

`hostname`: Hostname to run TLS check on (e.g. `valid-isrgrootx1.letsencrypt.org`).

`rootOrg`: Organization to check against the root certificate Organization (e.g. `Internet Security Research Group`).

`rootCN`: Name to check against the root certificate Common Name (e.g. `ISRG Root X1`). If not provided, root comparison will be skipped.

`response`: Expected site response; must be one of: `valid`, `revoked` or `expired`.

##### Example

```yaml
monitors:
  - 
    period: 1h
    kind: TLS
    settings:
      hostname: valid-isrgrootx1.letsencrypt.org
      rootOrg: "Internet Security Research Group"
      rootCN: "ISRG Root X1"
      response: valid
```

## Metrics

Observer provides the following metrics.

### Global Metrics

These metrics will always be available.

#### obs_monitors

Count of configured monitors.

**Labels:**

`kind`: Kind of Prober the monitor is configured to use.

`valid`: Bool indicating whether settings provided could be validated
for the `kind` of Prober specified.

#### obs_observations

**Labels:**

`name`: Name of the monitor.

`kind`: Kind of prober the monitor is configured to use.

`duration`: Duration of the probing in seconds.

`success`: Bool indicating whether the result of the probe attempt was
successful.

**Bucketed response times:**

This is configurable, see `buckets` under [root/schema](#schema).

### CRL Metrics

These metrics will be available whenever a valid CRL prober is configured.

#### obs_crl_this_update

Unix timestamp value (in seconds) of the thisUpdate field for a CRL.

**Labels:**

`url`: Url of the CRL

**Example Usage:**

This is a sample rule that alerts when a CRL has a thisUpdate timestamp in the future, signalling that something may have gone wrong during its creation:

```yaml
- alert: CRLThisUpdateInFuture
  expr: obs_crl_this_update{url="http://x1.c.lencr.org/"} > time()
  labels:
    severity: critical
  annotations:
    description: 'CRL thisUpdate is in the future'
```

#### obs_crl_next_update

Unix timestamp value (in seconds) of the nextUpdate field for a CRL.

**Labels:**

`url`: Url of the CRL

**Example Usage:**

This is a sample rule that alerts when a CRL has a nextUpdate timestamp in the past, signalling that the CRL was not updated on time:

```yaml
- alert: CRLNextUpdateInPast
  expr: obs_crl_next_update{url="http://x1.c.lencr.org/"} < time()
  labels:
    severity: critical
  annotations:
    description: 'CRL nextUpdate is in the past'
```

Another potentially useful rule would be to notify when nextUpdate is within X days from the current time, as a reminder that the update is coming up soon.

#### obs_crl_revoked_cert_count

Count of revoked certificates in a CRL.

**Labels:**

`url`: Url of the CRL

### TLS Metrics

These metrics will be available whenever a valid TLS prober is configured.

#### obs_tls_not_after

Unix timestamp value (in seconds) of the notAfter field for a subscriber certificate.

**Labels:**

`hostname`: Hostname of the site of the subscriber certificate

**Example Usage:**

This is a sample rule that alerts when a site has a notAfter timestamp indicating that the certificate will expire within the next 20 days:

```yaml
  - alert: CertExpiresSoonWarning
    annotations:
      description: "The certificate at {{ $labels.hostname }} expires within 20 days, on: {{ $value | humanizeTimestamp }}"
    expr: (obs_tls_not_after{hostname=~"^[^e][a-zA-Z]*-isrgrootx[12][.]letsencrypt[.]org"}) <= time() + 1728000
    for: 60m
    labels:
      severity: warning
```

#### obs_tls_reason

This is a count that increments by one for each resulting reason of a TSL check. The reason is `nil` if the TLS Prober returns `true` and one of the following otherwise: `internalError`, `ocspError`, `rootDidNotMatch`, `responseDidNotMatch`.

**Labels:**

`hostname`: Hostname of the site of the subscriber certificate
`reason`: The reason for TLS Probe returning false, and `nil` if it returns true

**Example Usage:**

This is a sample rule that alerts when TLS Prober returns false, providing insight on the reason for failure.

```yaml
  - alert: TLSCertCheckFailed
    annotations:
      description: "The TLS probe for {{ $labels.hostname }} failed for reason: {{ $labels.reason }}. This potentially violents CP 2.2."
    expr: (rate(obs_observations_count{success="false",name=~"[a-zA-Z]*-isrgrootx[12][.]letsencrypt[.]org"}[5m])) > 0
    for: 5m
    labels:
      severity: critical
```

## Development

### Starting Prometheus locally

Please note, this assumes you've installed a local Prometheus binary.

```shell
prometheus --config.file=boulder/test/prometheus/prometheus.yml
```

### Viewing metrics locally

When developing with a local Prometheus instance you can use this link
to view metrics: [link](http://0.0.0.0:9090)