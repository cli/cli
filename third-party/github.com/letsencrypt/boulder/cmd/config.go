package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc/resolver"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/identifier"
)

// PasswordConfig contains a path to a file containing a password.
type PasswordConfig struct {
	PasswordFile string `validate:"required"`
}

// Pass returns a password, extracted from the PasswordConfig's PasswordFile
func (pc *PasswordConfig) Pass() (string, error) {
	// Make PasswordConfigs optional, for backwards compatibility.
	if pc.PasswordFile == "" {
		return "", nil
	}
	contents, err := os.ReadFile(pc.PasswordFile)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(contents), "\n"), nil
}

// ServiceConfig contains config items that are common to all our services, to
// be embedded in other config structs.
type ServiceConfig struct {
	// DebugAddr is the address to run the /debug handlers on.
	DebugAddr string `validate:"omitempty,hostname_port"`
	GRPC      *GRPCServerConfig
	TLS       TLSConfig

	// HealthCheckInterval is the duration between deep health checks of the
	// service. Defaults to 5 seconds.
	HealthCheckInterval config.Duration `validate:"-"`
}

// DBConfig defines how to connect to a database. The connect string is
// stored in a file separate from the config, because it can contain a password,
// which we want to keep out of configs.
type DBConfig struct {
	// A file containing a connect URL for the DB.
	DBConnectFile string `validate:"required"`

	// MaxOpenConns sets the maximum number of open connections to the
	// database. If MaxIdleConns is greater than 0 and MaxOpenConns is
	// less than MaxIdleConns, then MaxIdleConns will be reduced to
	// match the new MaxOpenConns limit. If n < 0, then there is no
	// limit on the number of open connections.
	MaxOpenConns int `validate:"min=-1"`

	// MaxIdleConns sets the maximum number of connections in the idle
	// connection pool. If MaxOpenConns is greater than 0 but less than
	// MaxIdleConns, then MaxIdleConns will be reduced to match the
	// MaxOpenConns limit. If n < 0, no idle connections are retained.
	MaxIdleConns int `validate:"min=-1"`

	// ConnMaxLifetime sets the maximum amount of time a connection may
	// be reused. Expired connections may be closed lazily before reuse.
	// If d < 0, connections are not closed due to a connection's age.
	ConnMaxLifetime config.Duration `validate:"-"`

	// ConnMaxIdleTime sets the maximum amount of time a connection may
	// be idle. Expired connections may be closed lazily before reuse.
	// If d < 0, connections are not closed due to a connection's idle
	// time.
	ConnMaxIdleTime config.Duration `validate:"-"`
}

// URL returns the DBConnect URL represented by this DBConfig object, loading it
// from the file on disk. Leading and trailing whitespace is stripped.
func (d *DBConfig) URL() (string, error) {
	url, err := os.ReadFile(d.DBConnectFile)
	return strings.TrimSpace(string(url)), err
}

// SMTPConfig is deprecated.
// TODO(#8199): Delete this when it is removed from bad-key-revoker's config.
type SMTPConfig struct {
	PasswordConfig
	Server   string `validate:"required"`
	Port     string `validate:"required,numeric,min=1,max=65535"`
	Username string `validate:"required"`
}

// PAConfig specifies how a policy authority should connect to its
// database, what policies it should enforce, and what challenges
// it should offer.
type PAConfig struct {
	DBConfig    `validate:"-"`
	Challenges  map[core.AcmeChallenge]bool        `validate:"omitempty,dive,keys,oneof=http-01 dns-01 tls-alpn-01,endkeys"`
	Identifiers map[identifier.IdentifierType]bool `validate:"omitempty,dive,keys,oneof=dns ip,endkeys"`
}

// CheckChallenges checks whether the list of challenges in the PA config
// actually contains valid challenge names
func (pc PAConfig) CheckChallenges() error {
	if len(pc.Challenges) == 0 {
		return errors.New("empty challenges map in the Policy Authority config is not allowed")
	}
	for c := range pc.Challenges {
		if !c.IsValid() {
			return fmt.Errorf("invalid challenge in PA config: %s", c)
		}
	}
	return nil
}

// CheckIdentifiers checks whether the list of identifiers in the PA config
// actually contains valid identifier type names
func (pc PAConfig) CheckIdentifiers() error {
	for i := range pc.Identifiers {
		if !i.IsValid() {
			return fmt.Errorf("invalid identifier type in PA config: %s", i)
		}
	}
	return nil
}

// HostnamePolicyConfig specifies a file from which to load a policy regarding
// what hostnames to issue for.
type HostnamePolicyConfig struct {
	HostnamePolicyFile string `validate:"required"`
}

// TLSConfig represents certificates and a key for authenticated TLS.
type TLSConfig struct {
	CertFile string `validate:"required"`
	KeyFile  string `validate:"required"`
	// The CACertFile file may contain any number of root certificates and will
	// be deduplicated internally.
	CACertFile string `validate:"required"`
}

// Load reads and parses the certificates and key listed in the TLSConfig, and
// returns a *tls.Config suitable for either client or server use. The
// CACertFile file may contain any number of root certificates and will be
// deduplicated internally. Prometheus metrics for various certificate fields
// will be exported.
func (t *TLSConfig) Load(scope prometheus.Registerer) (*tls.Config, error) {
	if t == nil {
		return nil, fmt.Errorf("nil TLS section in config")
	}
	if t.CertFile == "" {
		return nil, fmt.Errorf("nil CertFile in TLSConfig")
	}
	if t.KeyFile == "" {
		return nil, fmt.Errorf("nil KeyFile in TLSConfig")
	}
	if t.CACertFile == "" {
		return nil, fmt.Errorf("nil CACertFile in TLSConfig")
	}
	caCertBytes, err := os.ReadFile(t.CACertFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert from %q: %s", t.CACertFile, err)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCertBytes); !ok {
		return nil, fmt.Errorf("parsing CA certs from %s failed", t.CACertFile)
	}
	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading key pair from %q and %q: %s",
			t.CertFile, t.KeyFile, err)
	}

	tlsNotBefore := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tlsconfig_notbefore_seconds",
			Help: "TLS certificate NotBefore field expressed as Unix epoch time",
		},
		[]string{"serial"})
	err = scope.Register(tlsNotBefore)
	if err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if errors.As(err, &are) {
			tlsNotBefore = are.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			return nil, err
		}
	}

	tlsNotAfter := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tlsconfig_notafter_seconds",
			Help: "TLS certificate NotAfter field expressed as Unix epoch time",
		},
		[]string{"serial"})
	err = scope.Register(tlsNotAfter)
	if err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if errors.As(err, &are) {
			tlsNotAfter = are.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			return nil, err
		}
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}

	serial := leaf.SerialNumber.String()
	tlsNotBefore.WithLabelValues(serial).Set(float64(leaf.NotBefore.Unix()))
	tlsNotAfter.WithLabelValues(serial).Set(float64(leaf.NotAfter.Unix()))

	return &tls.Config{
		RootCAs:      rootCAs,
		ClientCAs:    rootCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		// Set the only acceptable TLS to v1.3.
		MinVersion: tls.VersionTLS13,
	}, nil
}

// SyslogConfig defines the config for syslogging.
// 3 means "error", 4 means "warning", 6 is "info" and 7 is "debug".
// Configuring a given level causes all messages at that level and below to
// be logged.
type SyslogConfig struct {
	// When absent or zero, this causes no logs to be emitted on stdout/stderr.
	// Errors and warnings will be emitted on stderr if the configured level
	// allows.
	StdoutLevel int `validate:"min=-1,max=7"`
	// When absent or zero, this defaults to logging all messages of level 6
	// or below. To disable syslog logging entirely, set this to -1.
	SyslogLevel int `validate:"min=-1,max=7"`
}

// ServiceDomain contains the service and domain name the gRPC or bdns provider
// will use to construct a SRV DNS query to lookup backends.
type ServiceDomain struct {
	// Service is the service name to be used for SRV lookups. For example: if
	// record is 'foo.service.consul', then the Service is 'foo'.
	Service string `validate:"required"`

	// Domain is the domain name to be used for SRV lookups. For example: if the
	// record is 'foo.service.consul', then the Domain is 'service.consul'.
	Domain string `validate:"required"`
}

// GRPCClientConfig contains the information necessary to setup a gRPC client
// connection. The following field combinations are allowed:
//
// ServerIPAddresses, [Timeout]
// ServerAddress, DNSAuthority, [Timeout], [HostOverride]
// SRVLookup, DNSAuthority, [Timeout], [HostOverride], [SRVResolver]
// SRVLookups, DNSAuthority, [Timeout], [HostOverride], [SRVResolver]
type GRPCClientConfig struct {
	// DNSAuthority is a single <hostname|IPv4|[IPv6]>:<port> of the DNS server
	// to be used for resolution of gRPC backends. If the address contains a
	// hostname the gRPC client will resolve it via the system DNS. If the
	// address contains a port, the client will use it directly, otherwise port
	// 53 is used.
	DNSAuthority string `validate:"required_with=SRVLookup SRVLookups,omitempty,ip|hostname|hostname_port"`

	// SRVLookup contains the service and domain name the gRPC client will use
	// to construct a SRV DNS query to lookup backends. For example: if the
	// resource record is 'foo.service.consul', then the 'Service' is 'foo' and
	// the 'Domain' is 'service.consul'. The expected dNSName to be
	// authenticated in the server certificate would be 'foo.service.consul'.
	//
	// Note: The 'proto' field of the SRV record MUST contain 'tcp' and the
	// 'port' field MUST be a valid port. In a Consul configuration file you
	// would specify 'foo.service.consul' as:
	//
	// services {
	//   id      = "some-unique-id-1"
	//   name    = "foo"
	//   address = "10.77.77.77"
	//   port    = 8080
	//   tags    = ["tcp"]
	// }
	// services {
	//   id      = "some-unique-id-2"
	//   name    = "foo"
	//   address = "10.77.77.77"
	//   port    = 8180
	//   tags    = ["tcp"]
	// }
	//
	// If you've added the above to your Consul configuration file (and reloaded
	// Consul) then you should be able to resolve the following dig query:
	//
	// $ dig @10.77.77.10 -t SRV _foo._tcp.service.consul +short
	// 1 1 8080 0a585858.addr.dc1.consul.
	// 1 1 8080 0a4d4d4d.addr.dc1.consul.
	SRVLookup *ServiceDomain `validate:"required_without_all=SRVLookups ServerAddress ServerIPAddresses"`

	// SRVLookups allows you to pass multiple SRV records to the gRPC client.
	// The gRPC client will resolves each SRV record and use the results to
	// construct a list of backends to connect to. For more details, see the
	// documentation for the SRVLookup field. Note: while you can pass multiple
	// targets to the gRPC client using this field, all of the targets will use
	// the same HostOverride and TLS configuration.
	SRVLookups []*ServiceDomain `validate:"required_without_all=SRVLookup ServerAddress ServerIPAddresses"`

	// SRVResolver is an optional override to indicate that a specific
	// implementation of the SRV resolver should be used. The default is 'srv'
	// For more details, see the documentation in:
	// grpc/internal/resolver/dns/dns_resolver.go.
	SRVResolver string `validate:"excluded_with=ServerAddress ServerIPAddresses,isdefault|oneof=srv nonce-srv"`

	// ServerAddress is a single <hostname|IPv4|[IPv6]>:<port> or `:<port>` that
	// the gRPC client will, if necessary, resolve via DNS and then connect to.
	// If the address provided is 'foo.service.consul:8080' then the dNSName to
	// be authenticated in the server certificate would be 'foo.service.consul'.
	//
	// In a Consul configuration file you would specify 'foo.service.consul' as:
	//
	// services {
	//   id      = "some-unique-id-1"
	//   name    = "foo"
	//   address = "10.77.77.77"
	// }
	// services {
	//   id      = "some-unique-id-2"
	//   name    = "foo"
	//   address = "10.88.88.88"
	// }
	//
	// If you've added the above to your Consul configuration file (and reloaded
	// Consul) then you should be able to resolve the following dig query:
	//
	// $ dig A @10.77.77.10 foo.service.consul +short
	// 10.77.77.77
	// 10.88.88.88
	ServerAddress string `validate:"required_without_all=ServerIPAddresses SRVLookup SRVLookups,omitempty,hostname_port"`

	// ServerIPAddresses is a comma separated list of IP addresses, in the
	// format `<IPv4|[IPv6]>:<port>` or `:<port>`, that the gRPC client will
	// connect to. If the addresses provided are ["10.77.77.77", "10.88.88.88"]
	// then the iPAddress' to be authenticated in the server certificate would
	// be '10.77.77.77' and '10.88.88.88'.
	ServerIPAddresses []string `validate:"required_without_all=ServerAddress SRVLookup SRVLookups,omitempty,dive,hostname_port"`

	// HostOverride is an optional override for the dNSName the client will
	// verify in the certificate presented by the server.
	HostOverride string `validate:"excluded_with=ServerIPAddresses,omitempty,hostname"`
	Timeout      config.Duration

	// NoWaitForReady turns off our (current) default of setting grpc.WaitForReady(true).
	// This means if all of a GRPC client's backends are down, it will error immediately.
	// The current default, grpc.WaitForReady(true), means that if all of a GRPC client's
	// backends are down, it will wait until either one becomes available or the RPC
	// times out.
	NoWaitForReady bool
}

// MakeTargetAndHostOverride constructs the target URI that the gRPC client will
// connect to and the hostname (only for 'ServerAddress' and 'SRVLookup') that
// will be validated during the mTLS handshake. An error is returned if the
// provided configuration is invalid.
func (c *GRPCClientConfig) MakeTargetAndHostOverride() (string, string, error) {
	var hostOverride string
	if c.ServerAddress != "" {
		if c.ServerIPAddresses != nil || c.SRVLookup != nil {
			return "", "", errors.New(
				"both 'serverAddress' and 'serverIPAddresses' or 'SRVLookup' in gRPC client config. Only one should be provided",
			)
		}
		// Lookup backends using DNS A records.
		targetHost, _, err := net.SplitHostPort(c.ServerAddress)
		if err != nil {
			return "", "", err
		}

		hostOverride = targetHost
		if c.HostOverride != "" {
			hostOverride = c.HostOverride
		}
		return fmt.Sprintf("dns://%s/%s", c.DNSAuthority, c.ServerAddress), hostOverride, nil

	} else if c.SRVLookup != nil {
		if c.DNSAuthority == "" {
			return "", "", errors.New("field 'dnsAuthority' is required in gRPC client config with SRVLookup")
		}
		scheme, err := c.makeSRVScheme()
		if err != nil {
			return "", "", err
		}
		if c.ServerIPAddresses != nil {
			return "", "", errors.New(
				"both 'SRVLookup' and 'serverIPAddresses' in gRPC client config. Only one should be provided",
			)
		}
		// Lookup backends using DNS SRV records.
		targetHost := c.SRVLookup.Service + "." + c.SRVLookup.Domain

		hostOverride = targetHost
		if c.HostOverride != "" {
			hostOverride = c.HostOverride
		}
		return fmt.Sprintf("%s://%s/%s", scheme, c.DNSAuthority, targetHost), hostOverride, nil

	} else if c.SRVLookups != nil {
		if c.DNSAuthority == "" {
			return "", "", errors.New("field 'dnsAuthority' is required in gRPC client config with SRVLookups")
		}
		scheme, err := c.makeSRVScheme()
		if err != nil {
			return "", "", err
		}
		if c.ServerIPAddresses != nil {
			return "", "", errors.New(
				"both 'SRVLookups' and 'serverIPAddresses' in gRPC client config. Only one should be provided",
			)
		}
		// Lookup backends using multiple DNS SRV records.
		var targetHosts []string
		for _, s := range c.SRVLookups {
			targetHosts = append(targetHosts, s.Service+"."+s.Domain)
		}
		if c.HostOverride != "" {
			hostOverride = c.HostOverride
		}
		return fmt.Sprintf("%s://%s/%s", scheme, c.DNSAuthority, strings.Join(targetHosts, ",")), hostOverride, nil

	} else {
		if c.ServerIPAddresses == nil {
			return "", "", errors.New(
				"neither 'serverAddress', 'SRVLookup', 'SRVLookups' nor 'serverIPAddresses' in gRPC client config. One should be provided",
			)
		}
		// Specify backends as a list of IP addresses.
		return "static:///" + strings.Join(c.ServerIPAddresses, ","), "", nil
	}
}

// makeSRVScheme returns the scheme to use for SRV lookups. If the SRVResolver
// field is empty, it returns "srv". Otherwise it checks that the specified
// SRVResolver is registered with the gRPC runtime and returns it.
func (c *GRPCClientConfig) makeSRVScheme() (string, error) {
	if c.SRVResolver == "" {
		return "srv", nil
	}
	rb := resolver.Get(c.SRVResolver)
	if rb == nil {
		return "", fmt.Errorf("resolver %q is not registered", c.SRVResolver)
	}
	return c.SRVResolver, nil
}

// GRPCServerConfig contains the information needed to start a gRPC server.
type GRPCServerConfig struct {
	Address string `json:"address" validate:"omitempty,hostname_port"`
	// Services is a map of service names to configuration specific to that service.
	// These service names must match the service names advertised by gRPC itself,
	// which are identical to the names set in our gRPC .proto files prefixed by
	// the package names set in those files (e.g. "ca.CertificateAuthority").
	Services map[string]*GRPCServiceConfig `json:"services" validate:"required,dive,required"`
	// MaxConnectionAge specifies how long a connection may live before the server sends a GoAway to the
	// client. Because gRPC connections re-resolve DNS after a connection close,
	// this controls how long it takes before a client learns about changes to its
	// backends.
	// https://pkg.go.dev/google.golang.org/grpc/keepalive#ServerParameters
	MaxConnectionAge config.Duration `validate:"required"`
}

// GRPCServiceConfig contains the information needed to configure a gRPC service.
type GRPCServiceConfig struct {
	// ClientNames is the list of accepted gRPC client certificate SANs.
	// Connections from clients not in this list will be rejected by the
	// upstream listener, and RPCs from unlisted clients will be denied by the
	// server interceptor.
	ClientNames []string `json:"clientNames" validate:"min=1,dive,hostname,required"`
}

// OpenTelemetryConfig configures tracing via OpenTelemetry.
// To enable tracing, set a nonzero SampleRatio and configure an Endpoint
type OpenTelemetryConfig struct {
	// Endpoint to connect to with the OTLP protocol over gRPC.
	// It should be of the form "localhost:4317"
	//
	// It always connects over plaintext, and so is only intended to connect
	// to a local OpenTelemetry collector. This should not be used over an
	// insecure network.
	Endpoint string

	// SampleRatio is the ratio of new traces to head sample.
	// This only affects new traces without a parent with its own sampling
	// decision, and otherwise use the parent's sampling decision.
	//
	// Set to something between 0 and 1, where 1 is sampling all traces.
	// This is primarily meant as a pressure relief if the Endpoint we connect to
	// is being overloaded, and we otherwise handle sampling in the collectors.
	// See otel trace.ParentBased and trace.TraceIDRatioBased for details.
	SampleRatio float64
}

// OpenTelemetryHTTPConfig configures the otelhttp server tracing.
type OpenTelemetryHTTPConfig struct {
	// TrustIncomingSpans should only be set true if there's a trusted service
	// connecting to Boulder, such as a load balancer that's tracing-aware.
	// If false, the default, incoming traces won't be set as the parent.
	// See otelhttp.WithPublicEndpoint
	TrustIncomingSpans bool
}

// Options returns the otelhttp options for this configuration. They can be
// passed to otelhttp.NewHandler or Boulder's wrapper, measured_http.New.
func (c *OpenTelemetryHTTPConfig) Options() []otelhttp.Option {
	var options []otelhttp.Option
	if !c.TrustIncomingSpans {
		options = append(options, otelhttp.WithPublicEndpoint())
	}
	return options
}

// DNSProvider contains the configuration for a DNS provider in the bdns package
// which supports dynamic reloading of its backends.
type DNSProvider struct {
	// DNSAuthority is the single <hostname|IPv4|[IPv6]>:<port> of the DNS
	// server to be used for resolution of DNS backends. If the address contains
	// a hostname it will be resolved via the system DNS. If the port is left
	// unspecified it will default to '53'. If this field is left unspecified
	// the system DNS will be used for resolution of DNS backends.
	DNSAuthority string `validate:"required,ip|hostname|hostname_port"`

	// SRVLookup contains the service and domain name used to construct a SRV
	// DNS query to lookup DNS backends. 'Domain' is required. 'Service' is
	// optional and will be defaulted to 'dns' if left unspecified.
	//
	// Usage: If the resource record is 'unbound.service.consul', then the
	// 'Service' is 'unbound' and the 'Domain' is 'service.consul'. The expected
	// dNSName to be authenticated in the server certificate would be
	// 'unbound.service.consul'. The 'proto' field of the SRV record MUST
	// contain 'udp' and the 'port' field MUST be a valid port. In a Consul
	// configuration file you would specify 'unbound.service.consul' as:
	//
	// services {
	//   id      = "unbound-1" // Must be unique
	//   name    = "unbound"
	//   address = "10.77.77.77"
	//   port    = 8053
	//   tags    = ["udp"]
	// }
	//
	// services {
	//   id      = "unbound-2" // Must be unique
	//   name    = "unbound"
	//   address = "10.77.77.77"
	//   port    = 8153
	//   tags    = ["udp"]
	// }
	//
	// If you've added the above to your Consul configuration file (and reloaded
	// Consul) then you should be able to resolve the following dig query:
	//
	// $ dig @10.77.77.10 -t SRV _unbound._udp.service.consul +short
	// 1 1 8053 0a4d4d4d.addr.dc1.consul.
	// 1 1 8153 0a4d4d4d.addr.dc1.consul.
	SRVLookup ServiceDomain `validate:"required"`
}

// HMACKeyConfig specifies a path to a file containing a hexadecimal-encoded
// HMAC key. The key must represent exactly 256 bits (32 bytes) of random data
// to be suitable for use as a 256-bit hashing key (e.g., the output of `openssl
// rand -hex 32`).
type HMACKeyConfig struct {
	KeyFile string `validate:"required"`
}

// Load reads the HMAC key from the file, decodes it from hexadecimal, ensures
// it represents exactly 256 bits (32 bytes), and returns it as a byte slice.
func (hc *HMACKeyConfig) Load() ([]byte, error) {
	contents, err := os.ReadFile(hc.KeyFile)
	if err != nil {
		return nil, err
	}

	decoded, err := hex.DecodeString(strings.TrimSpace(string(contents)))
	if err != nil {
		return nil, fmt.Errorf("invalid hexadecimal encoding: %w", err)
	}

	if len(decoded) != 32 {
		return nil, fmt.Errorf(
			"validating HMAC key, must be exactly 256 bits (32 bytes) after decoding, got %d",
			len(decoded),
		)
	}
	return decoded, nil
}
