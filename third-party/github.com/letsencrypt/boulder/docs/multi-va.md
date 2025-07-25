# Multi-VA implementation

Boulder supports a multi-perspective validation feature intended to increase
resilience against local network hijacks and BGP attacks. It is currently
[deployed in a production
capacity](https://letsencrypt.org/2020/02/19/multi-perspective-validation.html)
by Let's Encrypt.

If you follow the [Development Instructions](https://github.com/letsencrypt/boulder#development)
to set up a Boulder environment in Docker and then change your `docker-compose.yml`'s
`BOULDER_CONFIG_DIR` to `test/config-next` instead of `test/config` you'll have
a Boulder environment configured with two primary VA instances (validation
requests are load balanced across the two) and two remote VA instances (each
primary VA will ask both remote VAs to perform matching validations for each
primary validation). Of course this is a development environment so both the
primary and remote VAs are all running on one host.

The `boulder-va` service ([here](https://github.com/letsencrypt/boulder/tree/main/cmd/boulder-va) and `remoteva` service ([here](https://github.com/letsencrypt/boulder/tree/main/cmd/remoteva)) are distinct pieces of software that utilize the same package ([here](https://github.com/letsencrypt/boulder/tree/main/va)).
The boulder-ra uses [the same RPC interface](https://github.com/letsencrypt/boulder/blob/ea231adc36746cce97f860e818c2cdf92f060543/va/proto/va.proto#L8-L10)
to ask for a primary validation as the primary VA uses to ask a remote VA for a
confirmation validation.

Primary VA instances contain a `"remoteVAs"` configuration element. If present
it specifies gRPC service addresses for `remoteva` instances to use as remote
VAs. There's also a handful of feature flags that control how the primary VAs
handle the remote VAs.

In the development environment with `config-next` the two primary VAs are `va1.service.consul:9092` and
`va2.service.consul:9092` and use
[`test/config-next/va.json`](https://github.com/letsencrypt/boulder/blob/ea231adc36746cce97f860e818c2cdf92f060543/test/config-next/va.json)
as their configuration. This config file specifies two `"remoteVA"s`,
`rva1.service.consul:9097` and `va2.service.consul:9098` and enforces
[that a maximum of 1 of the 2 remote VAs disagree](https://github.com/letsencrypt/boulder/blob/ea231adc36746cce97f860e818c2cdf92f060543/test/config-next/va.json#L44)
with the primary VA for all validations. The remote VA instances use
[`test/config-next/remoteva-a.json`](https://github.com/letsencrypt/boulder/blob/5c27eadb1db0605f380e41c8bd444a7f4ffe3c08/test/config-next/remoteva-a.json)
and
[`test/config-next/remoteva-b.json`](https://github.com/letsencrypt/boulder/blob/5c27eadb1db0605f380e41c8bd444a7f4ffe3c08/test/config-next/remoteva-b.json)
as their config files.

We require that almost all remote validation requests succeed; the exact number
is controlled by the VA based on the thresholds required by MPIC. If the number of
failing remote VAs exceeds that threshold, validation is terminated. If the
number of successful remote VAs is high enough that it would be impossible for
the outstanding remote VAs to exceed that threshold, validation immediately
succeeds.

There are some integration tests that test this end to end. The most relevant is
probably
[`test_http_multiva_threshold_fail`](https://github.com/letsencrypt/boulder/blob/ea231adc36746cce97f860e818c2cdf92f060543/test/v2_integration.py#L876-L908).
It tests that a HTTP-01 challenge made to a webserver that only gives the
correct key authorization to the primary VA and not the remotes will fail the
multi-perspective validation.
