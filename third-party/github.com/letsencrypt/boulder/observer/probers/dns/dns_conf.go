package probers

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/strictyaml"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	validQTypes = map[string]uint16{"A": 1, "TXT": 16, "AAAA": 28, "CAA": 257}
)

// DNSConf is exported to receive YAML configuration
type DNSConf struct {
	Proto   string `yaml:"protocol"`
	Server  string `yaml:"server"`
	Recurse bool   `yaml:"recurse"`
	QName   string `yaml:"query_name"`
	QType   string `yaml:"query_type"`
}

// Kind returns a name that uniquely identifies the `Kind` of `Configurer`.
func (c DNSConf) Kind() string {
	return "DNS"
}

// UnmarshalSettings constructs a DNSConf object from YAML as bytes.
func (c DNSConf) UnmarshalSettings(settings []byte) (probers.Configurer, error) {
	var conf DNSConf
	err := strictyaml.Unmarshal(settings, &conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

func (c DNSConf) validateServer() error {
	server := strings.Trim(strings.ToLower(c.Server), " ")
	// Ensure `server` contains a port.
	host, port, err := net.SplitHostPort(server)
	if err != nil || port == "" {
		return fmt.Errorf(
			"invalid `server`, %q, could not be split: %s", c.Server, err)
	}
	// Ensure `server` port is valid.
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf(
			"invalid `server`, %q, port must be a number", c.Server)
	}
	if portNum <= 0 || portNum > 65535 {
		return fmt.Errorf(
			"invalid `server`, %q, port number must be one in [1-65535]", c.Server)
	}
	// Ensure `server` is a valid FQDN or IPv4 / IPv6 address.
	IPv6 := net.ParseIP(host).To16()
	IPv4 := net.ParseIP(host).To4()
	FQDN := dns.IsFqdn(dns.Fqdn(host))
	if IPv6 == nil && IPv4 == nil && !FQDN {
		return fmt.Errorf(
			"invalid `server`, %q, is not an FQDN or IPv4 / IPv6 address", c.Server)
	}
	return nil
}

func (c DNSConf) validateProto() error {
	validProtos := []string{"udp", "tcp"}
	proto := strings.Trim(strings.ToLower(c.Proto), " ")
	for _, i := range validProtos {
		if proto == i {
			return nil
		}
	}
	return fmt.Errorf(
		"invalid `protocol`, got: %q, expected one in: %s", c.Proto, validProtos)
}

func (c DNSConf) validateQType() error {
	validQTypes = map[string]uint16{"A": 1, "TXT": 16, "AAAA": 28, "CAA": 257}
	qtype := strings.Trim(strings.ToUpper(c.QType), " ")
	q := make([]string, 0, len(validQTypes))
	for i := range validQTypes {
		q = append(q, i)
		if qtype == i {
			return nil
		}
	}
	return fmt.Errorf(
		"invalid `query_type`, got: %q, expected one in %s", c.QType, q)
}

// MakeProber constructs a `DNSProbe` object from the contents of the
// bound `DNSConf` object. If the `DNSConf` cannot be validated, an
// error appropriate for end-user consumption is returned instead.
func (c DNSConf) MakeProber(_ map[string]prometheus.Collector) (probers.Prober, error) {
	// validate `query_name`
	if !dns.IsFqdn(dns.Fqdn(c.QName)) {
		return nil, fmt.Errorf(
			"invalid `query_name`, %q is not an fqdn", c.QName)
	}

	// validate `server`
	err := c.validateServer()
	if err != nil {
		return nil, err
	}

	// validate `protocol`
	err = c.validateProto()
	if err != nil {
		return nil, err
	}

	// validate `query_type`
	err = c.validateQType()
	if err != nil {
		return nil, err
	}

	return DNSProbe{
		proto:   strings.Trim(strings.ToLower(c.Proto), " "),
		recurse: c.Recurse,
		qname:   c.QName,
		server:  c.Server,
		qtype:   validQTypes[strings.Trim(strings.ToUpper(c.QType), " ")],
	}, nil
}

// Instrument is a no-op to implement the `Configurer` interface.
func (c DNSConf) Instrument() map[string]prometheus.Collector {
	return nil
}

// init is called at runtime and registers `DNSConf`, a `Prober`
// `Configurer` type, as "DNS".
func init() {
	probers.Register(DNSConf{})
}
