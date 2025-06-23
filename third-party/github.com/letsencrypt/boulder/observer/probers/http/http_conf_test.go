package probers

import (
	"reflect"
	"testing"

	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/test"
	"gopkg.in/yaml.v3"
)

func TestHTTPConf_MakeProber(t *testing.T) {
	type fields struct {
		URL    string
		RCodes []int
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// valid
		{"valid fqdn valid rcode", fields{"http://example.com", []int{200}}, false},
		{"valid hostname valid rcode", fields{"example", []int{200}}, true},
		// invalid
		{"valid fqdn no rcode", fields{"http://example.com", nil}, true},
		{"valid fqdn invalid rcode", fields{"http://example.com", []int{1000}}, true},
		{"valid fqdn 1 invalid rcode", fields{"http://example.com", []int{200, 1000}}, true},
		{"bad fqdn good rcode", fields{":::::", []int{200}}, true},
		{"missing scheme", fields{"example.com", []int{200}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := HTTPConf{
				URL:    tt.fields.URL,
				RCodes: tt.fields.RCodes,
			}
			if _, err := c.MakeProber(nil); (err != nil) != tt.wantErr {
				t.Errorf("HTTPConf.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPConf_UnmarshalSettings(t *testing.T) {
	type fields struct {
		url       interface{}
		rcodes    interface{}
		useragent interface{}
		insecure  interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		want    probers.Configurer
		wantErr bool
	}{
		{"valid", fields{"google.com", []int{200}, "boulder_observer", false}, HTTPConf{"google.com", []int{200}, "boulder_observer", false}, false},
		{"invalid", fields{42, 42, 42, 42}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := probers.Settings{
				"url":       tt.fields.url,
				"rcodes":    tt.fields.rcodes,
				"useragent": tt.fields.useragent,
				"insecure":  tt.fields.insecure,
			}
			settingsBytes, _ := yaml.Marshal(settings)
			c := HTTPConf{}
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

func TestHTTPProberName(t *testing.T) {
	// Test with blank `useragent`
	proberYAML := `
url: https://www.google.com
rcodes: [ 200 ]
useragent: ""
insecure: true
`
	c := HTTPConf{}
	configurer, err := c.UnmarshalSettings([]byte(proberYAML))
	test.AssertNotError(t, err, "Got error for valid prober config")
	prober, err := configurer.MakeProber(nil)
	test.AssertNotError(t, err, "Got error for valid prober config")
	test.AssertEquals(t, prober.Name(), "https://www.google.com-[200]-letsencrypt/boulder-observer-http-client-insecure")

	// Test with custom `useragent`
	proberYAML = `
url: https://www.google.com
rcodes: [ 200 ]
useragent: fancy-custom-http-client
`
	c = HTTPConf{}
	configurer, err = c.UnmarshalSettings([]byte(proberYAML))
	test.AssertNotError(t, err, "Got error for valid prober config")
	prober, err = configurer.MakeProber(nil)
	test.AssertNotError(t, err, "Got error for valid prober config")
	test.AssertEquals(t, prober.Name(), "https://www.google.com-[200]-fancy-custom-http-client")

}
