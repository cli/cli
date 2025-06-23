package probers

import (
	"testing"

	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/test"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
)

func TestCRLConf_MakeProber(t *testing.T) {
	conf := CRLConf{}
	colls := conf.Instrument()
	badColl := prometheus.Collector(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "obs_crl_foo",
			Help: "Hmmm, this shouldn't be here...",
		},
		[]string{},
	))
	type fields struct {
		URL string
	}
	tests := []struct {
		name    string
		fields  fields
		colls   map[string]prometheus.Collector
		wantErr bool
	}{
		// valid
		{"valid fqdn", fields{"http://example.com"}, colls, false},
		{"valid fqdn with path", fields{"http://example.com/foo/bar"}, colls, false},
		{"valid hostname", fields{"http://example"}, colls, false},
		// invalid
		{"bad fqdn", fields{":::::"}, colls, true},
		{"missing scheme", fields{"example.com"}, colls, true},
		{
			"unexpected collector",
			fields{"http://example.com"},
			map[string]prometheus.Collector{"obs_crl_foo": badColl},
			true,
		},
		{
			"missing collectors",
			fields{"http://example.com"},
			map[string]prometheus.Collector{},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CRLConf{
				URL: tt.fields.URL,
			}
			p, err := c.MakeProber(tt.colls)
			if tt.wantErr {
				test.AssertError(t, err, "CRLConf.MakeProber()")
			} else {
				test.AssertNotError(t, err, "CRLConf.MakeProber()")

				test.AssertNotNil(t, p, "CRLConf.MakeProber(): nil prober")
				prober := p.(CRLProbe)
				test.AssertNotNil(t, prober.cThisUpdate, "CRLConf.MakeProber(): nil cThisUpdate")
				test.AssertNotNil(t, prober.cNextUpdate, "CRLConf.MakeProber(): nil cNextUpdate")
				test.AssertNotNil(t, prober.cCertCount, "CRLConf.MakeProber(): nil cCertCount")
			}
		})
	}
}

func TestCRLConf_UnmarshalSettings(t *testing.T) {
	type fields struct {
		url interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		want    probers.Configurer
		wantErr bool
	}{
		{"valid", fields{"google.com"}, CRLConf{"google.com"}, false},
		{"invalid (map)", fields{make(map[string]interface{})}, nil, true},
		{"invalid (list)", fields{make([]string, 0)}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := probers.Settings{
				"url": tt.fields.url,
			}
			settingsBytes, _ := yaml.Marshal(settings)
			t.Log(string(settingsBytes))
			c := CRLConf{}
			got, err := c.UnmarshalSettings(settingsBytes)
			if tt.wantErr {
				test.AssertError(t, err, "CRLConf.UnmarshalSettings()")
			} else {
				test.AssertNotError(t, err, "CRLConf.UnmarshalSettings()")
			}
			test.AssertDeepEquals(t, got, tt.want)
		})
	}
}
