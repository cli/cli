//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/cmd"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	nb "github.com/letsencrypt/boulder/grpc/noncebalancer"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/nonce"
	noncepb "github.com/letsencrypt/boulder/nonce/proto"
	"github.com/letsencrypt/boulder/test"
	"google.golang.org/grpc/status"
)

type nonceBalancerTestConfig struct {
	NotWFE struct {
		TLS                cmd.TLSConfig
		GetNonceService    *cmd.GRPCClientConfig
		RedeemNonceService *cmd.GRPCClientConfig
		NoncePrefixKey     cmd.PasswordConfig
	}
}

func TestNonceBalancer_NoBackendMatchingPrefix(t *testing.T) {
	t.Parallel()

	if !strings.Contains(os.Getenv("BOULDER_CONFIG_DIR"), "test/config-next") {
		t.Skip("Derived nonce prefixes are only configured in config-next")
	}

	// We're going to use a minimal nonce service client called "notwfe" which
	// masquerades as a wfe for the purpose of redeeming nonces.

	// Load the test config.
	var c nonceBalancerTestConfig
	err := cmd.ReadConfigFile("test/integration/testdata/nonce-client.json", &c)
	test.AssertNotError(t, err, "Could not read config file")

	tlsConfig, err := c.NotWFE.TLS.Load(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "Could not load TLS config")

	rncKey, err := c.NotWFE.NoncePrefixKey.Pass()
	test.AssertNotError(t, err, "Failed to load noncePrefixKey")

	clk := clock.New()

	redeemNonceConn, err := bgrpc.ClientSetup(c.NotWFE.RedeemNonceService, tlsConfig, metrics.NoopRegisterer, clk)
	test.AssertNotError(t, err, "Failed to load credentials and create gRPC connection to redeem nonce service")
	rnc := nonce.NewRedeemer(redeemNonceConn)

	// Attempt to redeem a nonce with a prefix that doesn't match any backends.
	ctx := context.WithValue(context.Background(), nonce.PrefixCtxKey{}, "12345678")
	ctx = context.WithValue(ctx, nonce.HMACKeyCtxKey{}, rncKey)
	_, err = rnc.Redeem(ctx, &noncepb.NonceMessage{Nonce: "0123456789"})

	// We expect to get a specific gRPC status error with code NotFound.
	gotRPCStatus, ok := status.FromError(err)
	test.Assert(t, ok, "Failed to convert error to status")
	test.AssertEquals(t, gotRPCStatus, nb.ErrNoBackendsMatchPrefix)
}
