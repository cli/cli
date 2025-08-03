package probers

import (
	"reflect"
	"testing"

	"github.com/letsencrypt/boulder/observer/probers"
	"github.com/letsencrypt/boulder/test"
	"gopkg.in/yaml.v3"
)

func TestDNSConf_validateServer(t *testing.T) {
	type fields struct {
		Server string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// ipv4 cases
		{"ipv4 with port", fields{"1.1.1.1:53"}, false},
		{"ipv4 without port", fields{"1.1.1.1"}, true},
		{"ipv4 port num missing", fields{"1.1.1.1:"}, true},
		{"ipv4 string for port", fields{"1.1.1.1:foo"}, true},
		{"ipv4 port out of range high", fields{"1.1.1.1:65536"}, true},
		{"ipv4 port out of range low", fields{"1.1.1.1:0"}, true},

		// ipv6 cases
		{"ipv6 with port", fields{"[2606:4700:4700::1111]:53"}, false},
		{"ipv6 without port", fields{"[2606:4700:4700::1111]"}, true},
		{"ipv6 port num missing", fields{"[2606:4700:4700::1111]:"}, true},
		{"ipv6 string for port", fields{"[2606:4700:4700::1111]:foo"}, true},
		{"ipv6 port out of range high", fields{"[2606:4700:4700::1111]:65536"}, true},
		{"ipv6 port out of range low", fields{"[2606:4700:4700::1111]:0"}, true},

		// hostname cases
		{"hostname with port", fields{"foo:53"}, false},
		{"hostname without port", fields{"foo"}, true},
		{"hostname port num missing", fields{"foo:"}, true},
		{"hostname string for port", fields{"foo:bar"}, true},
		{"hostname port out of range high", fields{"foo:65536"}, true},
		{"hostname port out of range low", fields{"foo:0"}, true},

		// fqdn cases
		{"fqdn with port", fields{"bar.foo.baz:53"}, false},
		{"fqdn without port", fields{"bar.foo.baz"}, true},
		{"fqdn port num missing", fields{"bar.foo.baz:"}, true},
		{"fqdn string for port", fields{"bar.foo.baz:bar"}, true},
		{"fqdn port out of range high", fields{"bar.foo.baz:65536"}, true},
		{"fqdn port out of range low", fields{"bar.foo.baz:0"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DNSConf{
				Server: tt.fields.Server,
			}
			err := c.validateServer()
			if tt.wantErr {
				test.AssertError(t, err, "DNSConf.validateServer() should have errored")
			} else {
				test.AssertNotError(t, err, "DNSConf.validateServer() shouldn't have errored")
			}
		})
	}
}

func TestDNSConf_validateQType(t *testing.T) {
	type fields struct {
		QType string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// valid
		{"A", fields{"A"}, false},
		{"AAAA", fields{"AAAA"}, false},
		{"TXT", fields{"TXT"}, false},
		// invalid
		{"AAA", fields{"AAA"}, true},
		{"TXTT", fields{"TXTT"}, true},
		{"D", fields{"D"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DNSConf{
				QType: tt.fields.QType,
			}
			err := c.validateQType()
			if tt.wantErr {
				test.AssertError(t, err, "DNSConf.validateQType() should have errored")
			} else {
				test.AssertNotError(t, err, "DNSConf.validateQType() shouldn't have errored")
			}
		})
	}
}

func TestDNSConf_validateProto(t *testing.T) {
	type fields struct {
		Proto string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// valid
		{"tcp", fields{"tcp"}, false},
		{"udp", fields{"udp"}, false},
		// invalid
		{"foo", fields{"foo"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DNSConf{
				Proto: tt.fields.Proto,
			}
			err := c.validateProto()
			if tt.wantErr {
				test.AssertError(t, err, "DNSConf.validateProto() should have errored")
			} else {
				test.AssertNotError(t, err, "DNSConf.validateProto() shouldn't have errored")
			}
		})
	}
}

func TestDNSConf_MakeProber(t *testing.T) {
	type fields struct {
		Proto   string
		Server  string
		Recurse bool
		QName   string
		QType   string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// valid
		{"valid", fields{"udp", "1.1.1.1:53", true, "google.com", "A"}, false},
		// invalid
		{"bad proto", fields{"can with string", "1.1.1.1:53", true, "google.com", "A"}, true},
		{"bad server", fields{"udp", "1.1.1.1:9000000", true, "google.com", "A"}, true},
		{"bad qtype", fields{"udp", "1.1.1.1:9000000", true, "google.com", "BAZ"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DNSConf{
				Proto:   tt.fields.Proto,
				Server:  tt.fields.Server,
				Recurse: tt.fields.Recurse,
				QName:   tt.fields.QName,
				QType:   tt.fields.QType,
			}
			_, err := c.MakeProber(nil)
			if tt.wantErr {
				test.AssertError(t, err, "DNSConf.MakeProber() should have errored")
			} else {
				test.AssertNotError(t, err, "DNSConf.MakeProber() shouldn't have errored")
			}
		})
	}
}

func TestDNSConf_UnmarshalSettings(t *testing.T) {
	type fields struct {
		protocol   interface{}
		server     interface{}
		recurse    interface{}
		query_name interface{}
		query_type interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		want    probers.Configurer
		wantErr bool
	}{
		{"valid", fields{"udp", "1.1.1.1:53", true, "google.com", "A"}, DNSConf{"udp", "1.1.1.1:53", true, "google.com", "A"}, false},
		{"invalid", fields{42, 42, 42, 42, 42}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := probers.Settings{
				"protocol":   tt.fields.protocol,
				"server":     tt.fields.server,
				"recurse":    tt.fields.recurse,
				"query_name": tt.fields.query_name,
				"query_type": tt.fields.query_type,
			}
			settingsBytes, _ := yaml.Marshal(settings)
			c := DNSConf{}
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
