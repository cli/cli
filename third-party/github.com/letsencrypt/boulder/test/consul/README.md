# Consul in Boulder
We use Consul in development mode (flag: `-dev`), which configures Consul as an
in-memory server and client with persistence disabled for ease of use.

## Configuring the Service Registry

- Open `./test/consul/config.hcl`
- Add a `services` stanza for each IP address and (optional) port combination
  you wish to have returned as an DNS record. The following stanza will return
  two records when resolving `foo-purger`.
  ([docs](https://www.consul.io/docs/discovery/services)).
  
  ```hcl
  services {
    id      = "foo-purger-a"
    name    = "foo-purger"
    address = "10.77.77.77"
    port    = 1338
  }

  services {
    id      = "foo-purger-b"
    name    = "foo-purger"
    address = "10.77.77.77"
    port    = 1438
  }
  ```
- To target individual `foo-purger`'s, add these additional `service` sections
  which allow resolving `foo-purger-1` and `foo-purger-2` respectively.

  ```hcl
  services {
    id      = "foo-purger-1"
    name    = "foo-purger-1"
    address = "10.77.77.77"
    port    = 1338
  }

  services {
    id      = "foo-purger-2"
    name    = "foo-purger-2"
    address = "10.77.77.77"
    port    = 1438
  }
  ```
- For RFC 2782 (SRV RR) lookups to work ensure you that you add a tag for the
  supported protocol (usually `"tcp"` and or `"udp"`) to the `tags` field.
  Consul implemented the `Proto` field as a tag filter for SRV RR lookups.
  For more information see the
  [docs](https://www.consul.io/docs/discovery/dns#rfc-2782-lookup).
  
  ```hcl
  services {
    id      = "foo-purger-a"
    name    = "foo-purger"
    address = "10.77.77.77"
    port    = 1338
    tags    = ["udp", "tcp"]
  }
  ...
  ```
- Services are **not** live-reloaded. You will need to cycle the container for
  every Service Registry change. 

## Accessing the web UI

### Linux

Consul should be accessible at http://10.77.77.10:8500.

### Mac

Docker desktop on macOS doesn't expose the bridge network adapter so you'll need
to add the following port lines (temporarily) to `docker-compose.yml`:

```yaml
  bconsul:
    ports:
      - 8500:8500 # forwards 127.0.0.1:8500 -> 10.77.77.10:8500
```

For testing DNS resolution locally using `dig` you'll need to add the following:
```yaml
  bconsul:
    ports:
      - 53:53/udp # forwards 127.0.0.1:53 -> 10.77.77.10:53
```

The next time you bring the container up you should be able to access the web UI
at http://127.0.0.1:8500.
