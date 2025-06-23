package probers

import (
	"reflect"
	"testing"

	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
)

func TestTLSConf_MakeProber(t *testing.T) {
	goodHostname, goodRootCN, goodResponse := "example.com", "ISRG Root X1", "valid"
	colls := TLSConf{}.Instrument()
	badColl := prometheus.Collector(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "obs_crl_foo",
			Help: "Hmmm, this shouldn't be here...",
		},
		[]string{},
	))
	type fields struct {
		Hostname string
		RootCN   string
		Response string
	}
	tests := []struct {
		name    string
		fields  fields
		colls   map[string]prometheus.Collector
		wantErr bool
	}{
		// valid
		{"valid hostname", fields{"example.com", goodRootCN, "valid"}, colls, false},
		{"valid hostname with path", fields{"example.com/foo/bar", "ISRG Root X2", "Revoked"}, colls, false},

		// invalid hostname
		{"bad hostname", fields{":::::", goodRootCN, goodResponse}, colls, true},
		{"included scheme", fields{"https://example.com", goodRootCN, goodResponse}, colls, true},

		// invalid response
		{"empty response", fields{goodHostname, goodRootCN, ""}, colls, true},
		{"unaccepted response", fields{goodHostname, goodRootCN, "invalid"}, colls, true},

		// invalid collector
		{
			"unexpected collector",
			fields{"http://example.com", goodRootCN, goodResponse},
			map[string]prometheus.Collector{"obs_crl_foo": badColl},
			true,
		},
		{
			"missing collectors",
			fields{"http://example.com", goodRootCN, goodResponse},
			map[string]prometheus.Collector{},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := TLSConf{
				Hostname: tt.fields.Hostname,
				RootCN:   tt.fields.RootCN,
				Response: tt.fields.Response,
			}
			if _, err := c.MakeProber(tt.colls); (err != nil) != tt.wantErr {
				t.Errorf("TLSConf.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTLSConf_UnmarshalSettings(t *testing.T) {
	type fields struct {
		hostname interface{}
		rootOrg  interface{}
		rootCN   interface{}
		response interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		want    probers.Configurer
		wantErr bool
	}{
		{"valid", fields{"google.com", "", "ISRG Root X1", "valid"}, TLSConf{"google.com", "", "ISRG Root X1", "valid"}, false},
		{"invalid hostname (map)", fields{make(map[string]interface{}), 42, 42, 42}, nil, true},
		{"invalid rootOrg (list)", fields{42, make([]string, 0), 42, 42}, nil, true},
		{"invalid response (list)", fields{42, 42, 42, make([]string, 0)}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := probers.Settings{
				"hostname": tt.fields.hostname,
				"rootOrg":  tt.fields.rootOrg,
				"rootCN":   tt.fields.rootCN,
				"response": tt.fields.response,
			}
			settingsBytes, _ := yaml.Marshal(settings)
			c := TLSConf{}
			got, err := c.UnmarshalSettings(settingsBytes)
			if (err != nil) != tt.wantErr {
				t.Errorf("DNSConf.UnmarshalSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DNSConf.UnmarshalSettings() = %v, want %v", got, tt.want)
			}
		})
	}
}
