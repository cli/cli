# Redis

We use Redis for OCSP. The Boulder dev environment stands up a two nodes. We use
the Ring client in the github.com/redis/go-redis package to consistently hash
our reads and writes across these two nodes. 

## Debugging

Our main tool for interacting with our OCSP storage in Redis is cmd/rocsp-tool.
However, sometimes if things aren't working right you might want to drop down a
level.

The first tool you might turn to is `redis-cli`. You probably don't
have redis-cli on your host, so we'll run it in a Docker container. We
also need to pass some specific arguments for TLS and authentication. There's a
script that handles all that for you: `test/redis-cli.sh`. First, make sure your
redis is running:

```shell
docker compose up boulder
```

Then, in a different window, run the following to connect to `bredis_1`:

```shell
./test/redis-cli.sh -h 10.77.77.2
```

Similarly, to connect to `bredis_2`:

```shell
./test/redis-cli.sh -h 10.77.77.3
```

You can pass any IP address for the -h (host) parameter. The full list of IP
addresses for Redis nodes is in `docker-compose.yml`. You can also pass other
redis-cli commandline parameters. They'll get passed through.

You may want to go a level deeper and communicate with a Redis node using the
Redis protocol. Here's the command to do that (run from the Boulder root):

```shell
openssl s_client -connect 10.77.77.2:4218 \
  -CAfile test/certs/ipki/minica.pem \
  -cert test/certs/ipki/localhost/cert.pem \
  -key test/certs/ipki/localhost/key.pem
```

Then, first thing when you connect, run `AUTH <user> <password>`. You can get a
list of usernames and passwords from test/redis.config.
