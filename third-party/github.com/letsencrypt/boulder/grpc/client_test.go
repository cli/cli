package grpc

import (
	"crypto/tls"
	"testing"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
	_ "google.golang.org/grpc/health"
)

func TestClientSetup(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *cmd.GRPCClientConfig
		expectTarget string
		wantErr      bool
	}{
		{"valid, address provided", &cmd.GRPCClientConfig{ServerAddress: "localhost:8080"}, "dns:///localhost:8080", false},
		{"valid, implicit localhost with port provided", &cmd.GRPCClientConfig{ServerAddress: ":8080"}, "dns:///:8080", false},
		{"valid, IPv6 address provided", &cmd.GRPCClientConfig{ServerAddress: "[::1]:8080"}, "dns:///[::1]:8080", false},
		{"valid, two addresses provided", &cmd.GRPCClientConfig{ServerIPAddresses: []string{"127.0.0.1:8080", "127.0.0.2:8080"}}, "static:///127.0.0.1:8080,127.0.0.2:8080", false},
		{"valid, two addresses provided, one has an implicit localhost, ", &cmd.GRPCClientConfig{ServerIPAddresses: []string{":8080", "127.0.0.2:8080"}}, "static:///:8080,127.0.0.2:8080", false},
		{"valid, two addresses provided, one is IPv6, ", &cmd.GRPCClientConfig{ServerIPAddresses: []string{"[::1]:8080", "127.0.0.2:8080"}}, "static:///[::1]:8080,127.0.0.2:8080", false},
		{"invalid, both address and addresses provided", &cmd.GRPCClientConfig{ServerAddress: "localhost:8080", ServerIPAddresses: []string{"127.0.0.1:8080"}}, "", true},
		{"invalid, no address or addresses provided", &cmd.GRPCClientConfig{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := ClientSetup(tt.cfg, &tls.Config{}, metrics.NoopRegisterer, clock.NewFake())
			if tt.wantErr {
				test.AssertError(t, err, "expected error, got nil")
			} else {
				test.AssertNotError(t, err, "unexpected error")
			}
			if tt.expectTarget != "" {
				test.AssertEquals(t, client.Target(), tt.expectTarget)
			}
		})
	}
}
