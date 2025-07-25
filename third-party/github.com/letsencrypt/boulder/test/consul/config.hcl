# Keep this file in sync with the ports bound in test/startservers.py

client_addr = "0.0.0.0"
bind_addr   = "10.77.77.10"
log_level   = "ERROR"
// When set, uses a subset of the agent's TLS configuration (key_file,
// cert_file, ca_file, ca_path, and server_name) to set up the client for HTTP
// or gRPC health checks. This allows services requiring 2-way TLS to be checked
// using the agent's credentials.
enable_agent_tls_for_checks = true
tls {
  defaults {
    ca_file         = "test/certs/ipki/minica.pem"
    ca_path         = "test/certs/ipki/minica-key.pem"
    cert_file       = "test/certs/ipki/consul.boulder/cert.pem"
    key_file        = "test/certs/ipki/consul.boulder/key.pem"
    verify_incoming = false
  }
}
ui_config {
  enabled = true
}
ports {
  dns      = 53
  grpc_tls = 8503
}

services {
  id      = "akamai-purger-a"
  name    = "akamai-purger"
  address = "10.77.77.77"
  port    = 9399
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "email-exporter-a"
  name    = "email-exporter"
  address = "10.77.77.77"
  port    = 9603
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "boulder-a"
  name    = "boulder"
  address = "10.77.77.77"
}

services {
  id      = "boulder-a"
  name    = "boulder"
  address = "10.77.77.77"
}

services {
  id      = "ca-a"
  name    = "ca"
  address = "10.77.77.77"
  port    = 9393
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "ca-b"
  name    = "ca"
  address = "10.77.77.77"
  port    = 9493
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "crl-storer-a"
  name    = "crl-storer"
  address = "10.77.77.77"
  port    = 9309
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "dns-a"
  name    = "dns"
  address = "10.77.77.77"
  port    = 8053
  tags    = ["udp"] // Required for SRV RR support in VA RVA.
}

services {
  id      = "dns-b"
  name    = "dns"
  address = "10.77.77.77"
  port    = 8054
  tags    = ["udp"] // Required for SRV RR support in VA RVA.
}

services {
  id      = "doh-a"
  name    = "doh"
  address = "10.77.77.77"
  port    = 8343
  tags    = ["tcp"]
}

services {
  id      = "doh-b"
  name    = "doh"
  address = "10.77.77.77"
  port    = 8443
  tags    = ["tcp"]
}

# Unlike most components, we have two completely independent nonce services,
# simulating two sets of nonce servers running in two different datacenters:
# taro and zinc.
services {
  id      = "nonce-taro-a"
  name    = "nonce-taro"
  address = "10.77.77.77"
  port    = 9301
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "nonce-taro-b"
  name    = "nonce-taro"
  address = "10.77.77.77"
  port    = 9501
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "nonce-zinc"
  name    = "nonce-zinc"
  address = "10.77.77.77"
  port    = 9401
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "publisher-a"
  name    = "publisher"
  address = "10.77.77.77"
  port    = 9391
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "publisher-b"
  name    = "publisher"
  address = "10.77.77.77"
  port    = 9491
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "ra-sct-provider-a"
  name    = "ra-sct-provider"
  address = "10.77.77.77"
  port    = 9594
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "ra-sct-provider-b"
  name    = "ra-sct-provider"
  address = "10.77.77.77"
  port    = 9694
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "ra-a"
  name    = "ra"
  address = "10.77.77.77"
  port    = 9394
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "ra-b"
  name    = "ra"
  address = "10.77.77.77"
  port    = 9494
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "rva1-a"
  name    = "rva1"
  address = "10.77.77.77"
  port    = 9397
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "rva1-b"
  name    = "rva1"
  address = "10.77.77.77"
  port    = 9498
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "rva1-c"
  name    = "rva1"
  address = "10.77.77.77"
  port    = 9499
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

# TODO(#5294) Remove rva2-a/b in favor of rva1-a/b
services {
  id      = "rva2-a"
  name    = "rva2"
  address = "10.77.77.77"
  port    = 9897
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "rva2-b"
  name    = "rva2"
  address = "10.77.77.77"
  port    = 9998
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "sa-a"
  name    = "sa"
  address = "10.77.77.77"
  port    = 9395
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
  checks = [
    {
      id              = "sa-a-grpc"
      name            = "sa-a-grpc"
      grpc            = "10.77.77.77:9395"
      grpc_use_tls    = true
      tls_server_name = "sa.boulder"
      tls_skip_verify = false
      interval        = "2s"
    },
    {
      id              = "sa-a-grpc-sa"
      name            = "sa-a-grpc-sa"
      grpc            = "10.77.77.77:9395/sa.StorageAuthority"
      grpc_use_tls    = true
      tls_server_name = "sa.boulder"
      tls_skip_verify = false
      interval        = "2s"
    },
    {
      id              = "sa-a-grpc-saro"
      name            = "sa-a-grpc-saro"
      grpc            = "10.77.77.77:9395/sa.StorageAuthorityReadOnly"
      grpc_use_tls    = true
      tls_server_name = "sa.boulder"
      tls_skip_verify = false
      interval        = "2s"
    }
  ]
}

services {
  id      = "sa-b"
  name    = "sa"
  address = "10.77.77.77"
  port    = 9495
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
  checks = [
    {
      id              = "sa-b-grpc"
      name            = "sa-b-grpc"
      grpc            = "10.77.77.77:9495"
      grpc_use_tls    = true
      tls_server_name = "sa.boulder"
      tls_skip_verify = false
      interval        = "2s"
    },
    {
      id              = "sa-b-grpc-sa"
      name            = "sa-b-grpc-sa"
      grpc            = "10.77.77.77:9495/sa.StorageAuthority"
      grpc_use_tls    = true
      tls_server_name = "sa.boulder"
      tls_skip_verify = false
      interval        = "2s"
    },
    {
      id              = "sa-b-grpc-saro"
      name            = "sa-b-grpc-saro"
      grpc            = "10.77.77.77:9495/sa.StorageAuthorityReadOnly"
      grpc_use_tls    = true
      tls_server_name = "sa.boulder"
      tls_skip_verify = false
      interval        = "2s"
    }
  ]
}

services {
  id      = "va-a"
  name    = "va"
  address = "10.77.77.77"
  port    = 9392
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "va-b"
  name    = "va"
  address = "10.77.77.77"
  port    = 9492
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

services {
  id      = "bredis3"
  name    = "redisratelimits"
  address = "10.77.77.4"
  port    = 4218
  tags    = ["tcp"] // Required for SRV RR support in DNS resolution.
}

services {
  id      = "bredis4"
  name    = "redisratelimits"
  address = "10.77.77.5"
  port    = 4218
  tags    = ["tcp"] // Required for SRV RR support in DNS resolution.
}

//
// The following services are used for testing the gRPC DNS resolver in
// test/integration/srv_resolver_test.go and
// test/integration/testdata/srv-resolver-config.json.
//

// CaseOne config will have 2 SRV records. The first will have 0 backends, the
// second will have 1.
services {
  id      = "case1a"
  name    = "case1a"
  address = "10.77.77.77"
  port    = 9301
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
  checks = [
    {
      id       = "case1a-failing"
      name     = "case1a-failing"
      http     = "http://localhost:12345" // invalid url
      method   = "GET"
      interval = "2s"
    }
  ]
}

services {
  id      = "case1b"
  name    = "case1b"
  address = "10.77.77.77"
  port    = 9401
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

// CaseTwo config will have 2 SRV records. The first will not be configured in
// Consul, the second will have 1 backend.
services {
  id      = "case2b"
  name    = "case2b"
  address = "10.77.77.77"
  port    = 9401
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
}

// CaseThree config will have 2 SRV records. Neither will be configured in
// Consul.


// CaseFour config will have 2 SRV records. Neither will have backends.
services {
  id      = "case4a"
  name    = "case4a"
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
  address = "10.77.77.77"
  port    = 9301
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
  checks = [
    {
      id       = "case4a-failing"
      name     = "case4a-failing"
      http     = "http://localhost:12345" // invalid url
      method   = "GET"
      interval = "2s"
    }
  ]
}

services {
  id      = "case4b"
  name    = "case4b"
  address = "10.77.77.77"
  port    = 9401
  tags    = ["tcp"] // Required for SRV RR support in gRPC DNS resolution.
  checks = [
    {
      id       = "case4b-failing"
      name     = "case4b-failing"
      http     = "http://localhost:12345" // invalid url
      method   = "GET"
      interval = "2s"
    }
  ]
}
