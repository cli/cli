package probers

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/strictyaml"
)

type MockConfigurer struct {
	Valid    bool            `yaml:"valid"`
	ErrMsg   string          `yaml:"errmsg"`
	PName    string          `yaml:"pname"`
	PKind    string          `yaml:"pkind"`
	PTook    config.Duration `yaml:"ptook"`
	PSuccess bool            `yaml:"psuccess"`
}

// Kind returns a name that uniquely identifies the `Kind` of `Configurer`.
func (c MockConfigurer) Kind() string {
	return "Mock"
}

func (c MockConfigurer) UnmarshalSettings(settings []byte) (probers.Configurer, error) {
	var conf MockConfigurer
	err := strictyaml.Unmarshal(settings, &conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

func (c MockConfigurer) MakeProber(_ map[string]prometheus.Collector) (probers.Prober, error) {
	if !c.Valid {
		return nil, errors.New("could not be validated")
	}
	return MockProber{c.PName, c.PKind, c.PTook, c.PSuccess}, nil
}

func (c MockConfigurer) Instrument() map[string]prometheus.Collector {
	return nil
}

func init() {
	probers.Register(MockConfigurer{})
}
