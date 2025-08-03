# Boulder - An ACME CA

[![Build Status](https://github.com/letsencrypt/boulder/actions/workflows/boulder-ci.yml/badge.svg?branch=main)](https://github.com/letsencrypt/boulder/actions/workflows/boulder-ci.yml?query=branch%3Amain)

This is an implementation of an ACME-based CA. The [ACME
protocol](https://github.com/ietf-wg-acme/acme/) allows the CA to automatically
verify that an applicant for a certificate actually controls an identifier, and
allows subscribers to issue and revoke certificates for the identifiers they
control. Boulder is the software that runs [Let's
Encrypt](https://letsencrypt.org).

## Contents

* [Overview](#overview)
* [Setting up Boulder](#setting-up-boulder)
  * [Development](#development)
  * [Working with Certbot](#working-with-certbot)
  * [Working with another ACME Client](#working-with-another-acme-client)
  * [Production](#production)
* [Contributing](#contributing)
* [License](#license)

## Overview

Boulder is divided into the following main components:

1. Web Front Ends (one per API version)
2. Registration Authority
3. Validation Authority
4. Certificate Authority
5. Storage Authority
6. Publisher
7. OCSP Responder
8. CRL Updater

This component model lets us separate the function of the CA by security
context. The Web Front End, Validation Authority, OCSP Responder and
Publisher need access to the Internet, which puts them at greater risk of
compromise. The Registration Authority can live without Internet
connectivity, but still needs to talk to the Web Front End and Validation
Authority. The Certificate Authority need only receive instructions from the
Registration Authority. All components talk to the SA for storage, so most
lines indicating SA RPCs are not shown here.

```text
                            CA ---------> Publisher
                             ^
                             |
       Subscriber -> WFE --> RA --> SA --> MariaDB
                             |               ^
Subscriber server <- VA <----+               |
                                             |
          Browser -------------------> OCSP Responder
```

Internally, the logic of the system is based around five types of objects:
accounts, authorizations, challenges, orders and certificates, mapping directly
to the resources of the same name in ACME. Requests from ACME clients result in
new objects and changes to objects. The Storage Authority maintains persistent
copies of the current set of objects.

Boulder uses gRPC for inter-component communication. For components that you
want to be remote, it is necessary to instantiate a "client" and "server" for
that component. The client implements the component's Go interface, while the
server has the actual logic for the component. A high level overview for this
communication model can be found in the [gRPC
documentation](https://www.grpc.io/docs/).

The full details of how the various ACME operations happen in Boulder are
laid out in
[DESIGN.md](https://github.com/letsencrypt/boulder/blob/main/docs/DESIGN.md).

## Setting up Boulder

### Development

Boulder has a Dockerfile and uses Docker Compose to make it easy to install
and set up all its dependencies. This is how the maintainers work on Boulder,
and is our main recommended way to run it for development/experimentation. It
is not suitable for use as a production environment.

While we aim to make Boulder easy to setup ACME client developers may find
[Pebble](https://github.com/letsencrypt/pebble), a miniature version of
Boulder, to be better suited for continuous integration and quick
experimentation.

We recommend setting git's [fsckObjects
setting](https://groups.google.com/forum/#!topic/binary-transparency/f-BI4o8HZW0/discussion)
before getting a copy of Boulder to have better integrity guarantees for
updates.

Clone the boulder repository:

```shell
git clone https://github.com/letsencrypt/boulder/
cd boulder
```

Additionally, make sure you have Docker Engine 1.13.0+ and Docker Compose
1.10.0+ installed. If you do not, you can follow Docker's [installation
instructions](https://docs.docker.com/compose/install/).

We recommend having **at least 2GB of RAM** available on your Docker host. In
practice using less RAM may result in the MariaDB container failing in
non-obvious ways.

To start Boulder in a Docker container, run:

```shell
docker compose up
```

To run our standard battery of tests (lints, unit, integration):

```shell
docker compose run --use-aliases boulder ./test.sh
```

To run all unit tests:

```shell
docker compose run --use-aliases boulder ./test.sh --unit
```

To run specific unit tests (example is of the ./va directory):

```shell
docker compose run --use-aliases boulder ./test.sh --unit --filter=./va
```

To run all integration tests:

```shell
docker compose run --use-aliases boulder ./test.sh --integration
```

To run specific integration tests (example runs TestAkamaiPurgerDrainQueueFails and TestWFECORS):

```shell
docker compose run --use-aliases boulder ./test.sh --filter TestAkamaiPurgerDrainQueueFails/TestWFECORS
```

To get a list of available integration tests:

```shell
docker compose run --use-aliases boulder ./test.sh --list-integration-tests
```

The configuration in docker-compose.yml mounts your boulder checkout at
/boulder so you can edit code on your host and it will be immediately
reflected inside the Docker containers run with `docker compose`.

If you have problems with Docker, you may want to try [removing all
containers and
volumes](https://www.digitalocean.com/community/tutorials/how-to-remove-docker-images-containers-and-volumes).

By default, Boulder uses a fake DNS resolver that resolves all hostnames to
127.0.0.1. This is suitable for running integration tests inside the Docker
container. If you want Boulder to be able to communicate with a client
running on your host instead, you should find your host's Docker IP with:

```shell
ifconfig docker0 | grep "inet addr:" | cut -d: -f2 | awk '{ print $1}'
```

And edit docker-compose.yml to change the `FAKE_DNS` environment variable to
match. This will cause Boulder's stubbed-out DNS resolver (`sd-test-srv`) to
respond to all A queries with the address in `FAKE_DNS`.

If you use a host-based firewall (e.g. `ufw` or `iptables`) make sure you allow
connections from the Docker instance to your host on the required validation
ports to your ACME client.

Alternatively, you can override the docker-compose.yml default with an
environmental variable using -e (replace 172.17.0.1 with the host IPv4
address found in the command above)

```shell
docker compose run --use-aliases -e FAKE_DNS=172.17.0.1 --service-ports boulder ./start.py
```

Running tests without the `./test.sh` wrapper:

Run all unit tests

```shell
docker compose run --use-aliases boulder go test -p 1 ./...
```

Run unit tests for a specific directory:

```shell
docker compose run --use-aliases boulder go test <DIRECTORY>
```

Run integration tests (omit `--filter <REGEX>` to run all):

```shell
docker compose run --use-aliases boulder python3 test/integration-test.py --chisel --gotest --filter <REGEX>
```

### Working with Certbot

Check out the Certbot client from https://github.com/certbot/certbot and
follow their setup instructions. Once you've got the client set up, you'll
probably want to run it against your local Boulder. There are a number of
command line flags that are necessary to run the client against a local
Boulder, and without root access. The simplest way to run the client locally
is to use a convenient alias for certbot (`certbot_test`) with a custom
`SERVER` environment variable:

```shell
SERVER=http://localhost:4001/directory certbot_test certonly --standalone -d test.example.com
```

Your local Boulder instance uses a fake DNS resolver that returns 127.0.0.1
for any query, so you can use any value for the -d flag. To return an answer
other than `127.0.0.1` change the Boulder `FAKE_DNS` environment variable to
another IP address.

### Working with another ACME Client

Once you have followed the Boulder development environment instructions and have
started the containers you will find the ACME endpoints exposed to your host at
the following URLs:

* ACME v2, HTTP: `http://localhost:4001/directory`
* ACME v2, HTTPS: `https://localhost:4431/directory`

To access the HTTPS versions of the endpoints you will need to configure your
ACME client software to use a CA truststore that contains the
`test/certs/ipki/minica.pem` CA certificate. See
[`test/certs/README.md`](https://github.com/letsencrypt/boulder/blob/main/test/certs/README.md)
for more information.

Your local Boulder instance uses a fake DNS resolver that returns 127.0.0.1
for any query, allowing you to issue certificates for any domain as if it
resolved to your localhost. To return an answer other than `127.0.0.1` change
the Boulder `FAKE_DNS` environment variable to another IP address.

Most often you will want to configure `FAKE_DNS` to point to your host
machine where you run an ACME client.

### Production

Boulder is custom built for Let's Encrypt and is intended only to support the
Web PKI and the CA/Browser forum's baseline requirements. In our experience
often Boulder is not the right fit for organizations that are evaluating it for
production usage. In most cases a centrally managed PKI that doesn't require
domain-authorization with ACME is a better choice. For this environment we
recommend evaluating a project other than Boulder.

We offer a brief [deployment and implementation
guide](https://github.com/letsencrypt/boulder/wiki/Deployment-&-Implementation-Guide)
that describes some of the required work and security considerations involved in
using Boulder in a production environment. As-is the docker based Boulder
development environment is **not suitable for
production usage**. It uses private key material that is publicly available,
exposes debug ports and is brittle to component failure.

While we are supportive of other organization's deploying Boulder in
a production setting we prioritize support and development work that favors
Let's Encrypt's mission. This means we may not be able to provide timely support
or accept pull-requests that deviate significantly from our first line goals. If
you've thoroughly evaluated the alternatives and Boulder is definitely the best
fit we're happy to answer questions to the best of our ability.

## Contributing

Please take a look at
[CONTRIBUTING.md](https://github.com/letsencrypt/boulder/blob/main/docs/CONTRIBUTING.md)
for our guidelines on submitting patches, code review process, code of conduct,
and various other tips related to working on the codebase.

## Code of Conduct

The code of conduct for everyone participating in this community in any capacity
is available for reference
[on the community forum](https://community.letsencrypt.org/guidelines).

## License

This project is licensed under the Mozilla Public License 2.0, the full text
of which can be found in the
[LICENSE.txt](https://github.com/letsencrypt/boulder/blob/main/LICENSE.txt)
file.
