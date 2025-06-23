package probers

import (
	"fmt"
	"net/url"

	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/strictyaml"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	nextUpdateName = "obs_crl_next_update"
	thisUpdateName = "obs_crl_this_update"
	certCountName  = "obs_crl_revoked_cert_count"
)

// CRLConf is exported to receive YAML configuration
type CRLConf struct {
	URL string `yaml:"url"`
}

// Kind returns a name that uniquely identifies the `Kind` of `Configurer`.
func (c CRLConf) Kind() string {
	return "CRL"
}

// UnmarshalSettings constructs a CRLConf object from YAML as bytes.
func (c CRLConf) UnmarshalSettings(settings []byte) (probers.Configurer, error) {
	var conf CRLConf
	err := strictyaml.Unmarshal(settings, &conf)

	if err != nil {
		return nil, err
	}
	return conf, nil
}

func (c CRLConf) validateURL() error {
	url, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf(
			"invalid 'url', got: %q, expected a valid url", c.URL)
	}
	if url.Scheme == "" {
		return fmt.Errorf(
			"invalid 'url', got: %q, missing scheme", c.URL)
	}
	return nil
}

// MakeProber constructs a `CRLProbe` object from the contents of the
// bound `CRLConf` object. If the `CRLConf` cannot be validated, an
// error appropriate for end-user consumption is returned instead.
func (c CRLConf) MakeProber(collectors map[string]prometheus.Collector) (probers.Prober, error) { // validate `url` err := c.validateURL()
	// validate `url`
	err := c.validateURL()
	if err != nil {
		return nil, err
	}

	// validate the prometheus collectors that were passed in
	coll, ok := collectors[nextUpdateName]
	if !ok {
		return nil, fmt.Errorf("crl prober did not receive collector %q", nextUpdateName)
	}
	nextUpdateColl, ok := coll.(*prometheus.GaugeVec)
	if !ok {
		return nil, fmt.Errorf("crl prober received collector %q of wrong type, got: %T, expected *prometheus.GaugeVec", nextUpdateName, coll)
	}

	coll, ok = collectors[thisUpdateName]
	if !ok {
		return nil, fmt.Errorf("crl prober did not receive collector %q", thisUpdateName)
	}
	thisUpdateColl, ok := coll.(*prometheus.GaugeVec)
	if !ok {
		return nil, fmt.Errorf("crl prober received collector %q of wrong type, got: %T, expected *prometheus.GaugeVec", thisUpdateName, coll)
	}

	coll, ok = collectors[certCountName]
	if !ok {
		return nil, fmt.Errorf("crl prober did not receive collector %q", certCountName)
	}
	certCountColl, ok := coll.(*prometheus.GaugeVec)
	if !ok {
		return nil, fmt.Errorf("crl prober received collector %q of wrong type, got: %T, expected *prometheus.GaugeVec", certCountName, coll)
	}

	return CRLProbe{c.URL, nextUpdateColl, thisUpdateColl, certCountColl}, nil
}

// Instrument constructs any `prometheus.Collector` objects the `CRLProbe` will
// need to report its own metrics. A map is returned containing the constructed
// objects, indexed by the name of the prometheus metric. If no objects were
// constructed, nil is returned.
func (c CRLConf) Instrument() map[string]prometheus.Collector {
	nextUpdate := prometheus.Collector(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: nextUpdateName,
			Help: "CRL nextUpdate Unix timestamp in seconds",
		}, []string{"url"},
	))
	thisUpdate := prometheus.Collector(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: thisUpdateName,
			Help: "CRL thisUpdate Unix timestamp in seconds",
		}, []string{"url"},
	))
	certCount := prometheus.Collector(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: certCountName,
			Help: "number of certificates revoked in CRL",
		}, []string{"url"},
	))
	return map[string]prometheus.Collector{
		nextUpdateName: nextUpdate,
		thisUpdateName: thisUpdate,
		certCountName:  certCount,
	}
}

// init is called at runtime and registers `CRLConf`, a `Prober`
// `Configurer` type, as "CRL".
func init() {
	probers.Register(CRLConf{})
}
