package tcp

import (
	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/strictyaml"
	"github.com/prometheus/client_golang/prometheus"
)

// TCPConf is exported to receive YAML configuration.
type TCPConf struct {
	Hostport string `yaml:"hostport"`
}

// Kind returns a name that uniquely identifies the `Kind` of `Configurer`.
func (c TCPConf) Kind() string {
	return "TCP"
}

// UnmarshalSettings takes YAML as bytes and unmarshals it to the to an
// TCPConf object.
func (c TCPConf) UnmarshalSettings(settings []byte) (probers.Configurer, error) {
	var conf TCPConf
	err := strictyaml.Unmarshal(settings, &conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// MakeProber constructs a `TCPPProbe` object from the contents of the
// bound `TCPPConf` object.
func (c TCPConf) MakeProber(_ map[string]prometheus.Collector) (probers.Prober, error) {
	return TCPProbe{c.Hostport}, nil
}

// Instrument is a no-op to implement the `Configurer` interface.
func (c TCPConf) Instrument() map[string]prometheus.Collector {
	return nil
}

// init is called at runtime and registers `TCPConf`, a `Prober`
// `Configurer` type, as "TCP".
func init() {
	probers.Register(TCPConf{})
}
