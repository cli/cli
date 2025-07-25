package grpc

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
	"google.golang.org/grpc/resolver"
)

func Test_parseResolverIPAddress(t *testing.T) {
	tests := []struct {
		name         string
		addr         string
		expectTarget *resolver.Address
		wantErr      bool
	}{
		{"valid, IPv4 address", "127.0.0.1:1337", &resolver.Address{Addr: "127.0.0.1:1337", ServerName: "127.0.0.1:1337"}, false},
		{"valid, IPv6 address", "[::1]:1337", &resolver.Address{Addr: "[::1]:1337", ServerName: "[::1]:1337"}, false},
		{"valid, port only", ":1337", &resolver.Address{Addr: "127.0.0.1:1337", ServerName: "127.0.0.1:1337"}, false},
		{"invalid, hostname address", "localhost:1337", nil, true},
		{"invalid, IPv6 address, no brackets", "::1:1337", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResolverIPAddress(tt.addr)
			if tt.wantErr {
				test.AssertError(t, err, "expected error, got nil")
			} else {
				test.AssertNotError(t, err, "unexpected error")
			}
			test.AssertDeepEquals(t, got, tt.expectTarget)
		})
	}
}
