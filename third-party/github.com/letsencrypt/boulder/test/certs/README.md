# Test keys and certificates

## Dynamically-Generated PKIs

This directory contains scripts and programs which generate PKIs (collections of
keys and certificates) for use in our integration tests. Each PKI has its own
subdirectory. The scripts do not regenerate a directory if it already exists, to
allow the generated files to be re-used across many runs on a developer's
machine. To force the scripts to regenerate a PKI, simply delete its whole
directory.

This script is invoked automatically by the `bsetup` container in our docker
compose system. It is invoked automatically by `t.sh` and `tn.sh`. If you want
to run it manually, the expected way to do so is:

```sh
$ docker compose up bsetup
[+] Running 0/1
Attaching to bsetup-1
bsetup-1  | Generating ipki/...
bsetup-1  | Generating webpki/...
bsetup-1 exited with code 0
```

To add new certificates to an existing PKI, edit the script which generates that
PKI's subdirectory. To add a whole new PKI, create a new generation script,
execute that script from this directory's top-level `generate.sh`, and add the
new subdirectory to this directory's `.gitignore` file.

### webpki

The "webpki" PKI emulates our publicly-trusted hierarchy. It consists of RSA and
ECDSA roots, several intermediates and cross-signed intermediates, and CRLs.
These certificates and their keys are generated using the `ceremony` tool. The
private keys are stored in SoftHSM in the `.softhsm-tokens` subdirectory.

This PKI is loaded by the CA, RA, and other components. It is used as the
issuance hierarchy for all end-entity certificates issued as part of the
integration tests.

### ipki

The "ipki" PKI emulates our internal PKI that the various Boulder services use
to authenticate each other when establishing gRPC connections. It includes one
certificate for each service which participates in our gRPC cluster. Some of
these certificates (for the services that we run multiple copies of) have
multiple names, so the same certificate can be loaded by each copy of that
service.

It also contains some non-gRPC certificates which are nonetheless serving the
role of internal authentication between Let's Encrypt components:

- The IP-address certificate used by challtestsrv (which acts as the integration
  test environment's recursive resolver) for DoH handshakes.
- The certificate presented by the test redis cluster.
- The certificate presented by the WFE's API TLS handler (which is usually
  behind some other load-balancer like nginx).

This PKI is loaded by virtually every Boulder component.

**Note:** the minica issuer certificate and the "localhost" end-entity
certificate are also used by several rocsp and ratelimit unit tests. The tests
use these certificates to authenticate to the docker-compose redis cluster, and
therefore cannot succeed outside of the docker environment anyway, so a
dependency on the ipki hierarchy having been generated does not break them
further.

## Other Test PKIs

A variety of other PKIs (collections of keys and certificates) exist in this
repository for the sake of unit and integration testing. We list them here as a
TODO-list of PKIs to remove and clean up:

- unit test hierarchy: the //test/hierarchy/ directory holds a collection of
  certificates used by unit tests which want access to realistic issuer certs
  but don't want to rely on the //test/certs/webpki directory being generated.
  These should be replaced by certs which the unit tests dynamically generate
  in-memory, rather than loading from disk.
- unit test mocks: //test/test-key-5.der and //wfe2/wfe_test.go contain keys and
  certificates which are used to elicit specific behavior from //mocks/mocks.go.
  These should be replaced with dynamically-generated keys and more flexible
  mocks.
