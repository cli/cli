package observer

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/observer/probers"
)

// MonConf is exported to receive YAML configuration in `ObsConf`.
type MonConf struct {
	Period   config.Duration  `yaml:"period"`
	Kind     string           `yaml:"kind" validate:"required,oneof=DNS HTTP CRL TLS TCP"`
	Settings probers.Settings `yaml:"settings" validate:"min=1,dive"`
}

// validatePeriod ensures the received `Period` field is at least 1µs.
func (c *MonConf) validatePeriod() error {
	if c.Period.Duration < 1*time.Microsecond {
		return errors.New("period must be at least 1µs")
	}
	return nil
}

// unmarshalConfigurer constructs a `Configurer` by marshaling the
// value of the `Settings` field back to bytes, then passing it to the
// `UnmarshalSettings` method of the `Configurer` type specified by the
// `Kind` field.
func (c MonConf) unmarshalConfigurer() (probers.Configurer, error) {
	configurer, err := probers.GetConfigurer(c.Kind)
	if err != nil {
		return nil, err
	}
	settings, _ := yaml.Marshal(c.Settings)
	configurer, err = configurer.UnmarshalSettings(settings)
	if err != nil {
		return nil, err
	}
	return configurer, nil
}

// makeMonitor constructs a `monitor` object from the contents of the
// bound `MonConf`. If the `MonConf` cannot be validated, an error
// appropriate for end-user consumption is returned instead.
func (c MonConf) makeMonitor(collectors map[string]prometheus.Collector) (*monitor, error) {
	err := c.validatePeriod()
	if err != nil {
		return nil, err
	}
	probeConf, err := c.unmarshalConfigurer()
	if err != nil {
		return nil, err
	}
	prober, err := probeConf.MakeProber(collectors)
	if err != nil {
		return nil, err
	}
	return &monitor{c.Period.Duration, prober}, nil
}
