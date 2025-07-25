package bdns

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func Test_validateServerAddress(t *testing.T) {
	type args struct {
		server string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// ipv4 cases
		{"ipv4 with port", args{"1.1.1.1:53"}, false},
		// sad path
		{"ipv4 without port", args{"1.1.1.1"}, true},
		{"ipv4 port num missing", args{"1.1.1.1:"}, true},
		{"ipv4 string for port", args{"1.1.1.1:foo"}, true},
		{"ipv4 port out of range high", args{"1.1.1.1:65536"}, true},
		{"ipv4 port out of range low", args{"1.1.1.1:0"}, true},

		// ipv6 cases
		{"ipv6 with port", args{"[2606:4700:4700::1111]:53"}, false},
		// sad path
		{"ipv6 sans brackets", args{"2606:4700:4700::1111:53"}, true},
		{"ipv6 without port", args{"[2606:4700:4700::1111]"}, true},
		{"ipv6 port num missing", args{"[2606:4700:4700::1111]:"}, true},
		{"ipv6 string for port", args{"[2606:4700:4700::1111]:foo"}, true},
		{"ipv6 port out of range high", args{"[2606:4700:4700::1111]:65536"}, true},
		{"ipv6 port out of range low", args{"[2606:4700:4700::1111]:0"}, true},

		// hostname cases
		{"hostname with port", args{"foo:53"}, false},
		// sad path
		{"hostname without port", args{"foo"}, true},
		{"hostname port num missing", args{"foo:"}, true},
		{"hostname string for port", args{"foo:bar"}, true},
		{"hostname port out of range high", args{"foo:65536"}, true},
		{"hostname port out of range low", args{"foo:0"}, true},

		// fqdn cases
		{"fqdn with port", args{"bar.foo.baz:53"}, false},
		// sad path
		{"fqdn without port", args{"bar.foo.baz"}, true},
		{"fqdn port num missing", args{"bar.foo.baz:"}, true},
		{"fqdn string for port", args{"bar.foo.baz:bar"}, true},
		{"fqdn port out of range high", args{"bar.foo.baz:65536"}, true},
		{"fqdn port out of range low", args{"bar.foo.baz:0"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServerAddress(tt.args.server)
			if (err != nil) != tt.wantErr {
				t.Errorf("formatServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_resolveDNSAuthority(t *testing.T) {
	type want struct {
		host string
		port string
	}
	tests := []struct {
		name    string
		target  string
		want    want
		wantErr bool
	}{
		{"IP4 with port", "10.10.10.10:53", want{"10.10.10.10", "53"}, false},
		{"IP4 without port", "10.10.10.10", want{"10.10.10.10", "53"}, false},
		{"IP6 with port and brackets", "[2606:4700:4700::1111]:53", want{"2606:4700:4700::1111", "53"}, false},
		{"IP6 without port", "2606:4700:4700::1111", want{"2606:4700:4700::1111", "53"}, false},
		{"IP6 with brackets without port", "[2606:4700:4700::1111]", want{"2606:4700:4700::1111", "53"}, false},
		{"hostname with port", "localhost:53", want{"localhost", "53"}, false},
		{"hostname without port", "localhost", want{"localhost", "53"}, false},
		{"only port", ":53", want{"localhost", "53"}, false},
		{"hostname with no port after colon", "localhost:", want{"", ""}, true},
		{"IP4 with no port after colon", "10.10.10.10:", want{"", ""}, true},
		{"IP6 with no port after colon", "[2606:4700:4700::1111]:", want{"", ""}, true},
		{"no hostname or port", "", want{"", ""}, true},
		{"invalid addr", "foo:bar:baz", want{"", ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, gotErr := ParseTarget(tt.target, "53")
			test.AssertEquals(t, gotHost, tt.want.host)
			test.AssertEquals(t, gotPort, tt.want.port)
			if tt.wantErr {
				test.AssertError(t, gotErr, "expected error")
			} else {
				test.AssertNotError(t, gotErr, "unexpected error")
			}
		})
	}
}
