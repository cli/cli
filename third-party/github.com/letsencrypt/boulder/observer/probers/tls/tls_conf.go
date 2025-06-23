package probers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/strictyaml"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	notAfterName  = "obs_tls_not_after"
	notBeforeName = "obs_tls_not_before"
	reasonName    = "obs_tls_reason"
)

// TLSConf is exported to receive YAML configuration.
type TLSConf struct {
	Hostname string `yaml:"hostname"`
	RootOrg  string `yaml:"rootOrg"`
	RootCN   string `yaml:"rootCN"`
	Response string `yaml:"response"`
}

// Kind returns a name that uniquely identifies the `Kind` of `Configurer`.
func (c TLSConf) Kind() string {
	return "TLS"
}

// UnmarshalSettings takes YAML as bytes and unmarshals it to the to an TLSConf
// object.
func (c TLSConf) UnmarshalSettings(settings []byte) (probers.Configurer, error) {
	var conf TLSConf
	err := strictyaml.Unmarshal(settings, &conf)
	if err != nil {
		return nil, err
	}

	return conf, nil
}

func (c TLSConf) validateHostname() error {
	url, err := url.Parse(c.Hostname)
	if err != nil {
		return fmt.Errorf(
			"invalid 'hostname', got %q, expected a valid hostname: %s", c.Hostname, err)
	}

	if url.Scheme != "" {
		return fmt.Errorf(
			"invalid 'hostname', got: %q, should not include scheme", c.Hostname)
	}

	return nil
}

func (c TLSConf) validateResponse() error {
	acceptable := []string{"valid", "expired", "revoked"}
	for _, a := range acceptable {
		if strings.ToLower(c.Response) == a {
			return nil
		}
	}

	return fmt.Errorf(
		"invalid `response`, got %q. Must be one of %s", c.Response, acceptable)
}

// MakeProber constructs a `TLSProbe` object from the contents of the bound
// `TLSConf` object. If the `TLSConf` cannot be validated, an error appropriate
// for end-user consumption is returned instead.
func (c TLSConf) MakeProber(collectors map[string]prometheus.Collector) (probers.Prober, error) {
	// Validate `hostname`
	err := c.validateHostname()
	if err != nil {
		return nil, err
	}

	// Valid `response`
	err = c.validateResponse()
	if err != nil {
		return nil, err
	}

	// Validate the Prometheus collectors that were passed in
	coll, ok := collectors[notAfterName]
	if !ok {
		return nil, fmt.Errorf("tls prober did not receive collector %q", notAfterName)
	}

	notAfterColl, ok := coll.(*prometheus.GaugeVec)
	if !ok {
		return nil, fmt.Errorf("tls prober received collector %q of wrong type, got: %T, expected *prometheus.GaugeVec", notAfterName, coll)
	}

	coll, ok = collectors[notBeforeName]
	if !ok {
		return nil, fmt.Errorf("tls prober did not receive collector %q", notBeforeName)
	}

	notBeforeColl, ok := coll.(*prometheus.GaugeVec)
	if !ok {
		return nil, fmt.Errorf("tls prober received collector %q of wrong type, got: %T, expected *prometheus.GaugeVec", notBeforeName, coll)
	}

	coll, ok = collectors[reasonName]
	if !ok {
		return nil, fmt.Errorf("tls prober did not receive collector %q", reasonName)
	}

	reasonColl, ok := coll.(*prometheus.CounterVec)
	if !ok {
		return nil, fmt.Errorf("tls prober received collector %q of wrong type, got: %T, expected *prometheus.CounterVec", reasonName, coll)
	}

	return TLSProbe{c.Hostname, c.RootOrg, c.RootCN, strings.ToLower(c.Response), notAfterColl, notBeforeColl, reasonColl}, nil
}

// Instrument constructs any `prometheus.Collector` objects the `TLSProbe` will
// need to report its own metrics. A map is returned containing the constructed
// objects, indexed by the name of the Promtheus metric.  If no objects were
// constructed, nil is returned.
func (c TLSConf) Instrument() map[string]prometheus.Collector {
	notBefore := prometheus.Collector(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: notBeforeName,
			Help: "Certificate notBefore value as a Unix timestamp in seconds",
		}, []string{"hostname"},
	))
	notAfter := prometheus.Collector(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: notAfterName,
			Help: "Certificate notAfter value as a Unix timestamp in seconds",
		}, []string{"hostname"},
	))
	reason := prometheus.Collector(prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: reasonName,
			Help: fmt.Sprintf("Reason for TLS Prober check failure. Can be one of %s", getReasons()),
		}, []string{"hostname", "reason"},
	))
	return map[string]prometheus.Collector{
		notAfterName:  notAfter,
		notBeforeName: notBefore,
		reasonName:    reason,
	}
}

// init is called at runtime and registers `TLSConf`, a `Prober` `Configurer`
// type, as "TLS".
func init() {
	probers.Register(TLSConf{})
}
