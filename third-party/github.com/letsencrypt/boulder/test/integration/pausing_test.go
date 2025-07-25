//go:build integration

package integration

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"
	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/metrics"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
)

func TestIdentifiersPausedForAccount(t *testing.T) {
	t.Parallel()

	tlsCerts := &cmd.TLSConfig{
		CACertFile: "test/certs/ipki/minica.pem",
		CertFile:   "test/certs/ipki/ra.boulder/cert.pem",
		KeyFile:    "test/certs/ipki/ra.boulder/key.pem",
	}
	tlsConf, err := tlsCerts.Load(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "Failed to load TLS config")
	saConn, err := bgrpc.ClientSetup(
		&cmd.GRPCClientConfig{
			DNSAuthority: "consul.service.consul",
			SRVLookup: &cmd.ServiceDomain{
				Service: "sa",
				Domain:  "service.consul",
			},

			Timeout:        config.Duration{Duration: 5 * time.Second},
			NoWaitForReady: true,
			HostOverride:   "sa.boulder",
		},
		tlsConf,
		metrics.NoopRegisterer,
		clock.NewFake(),
	)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to SA")
	saClient := sapb.NewStorageAuthorityClient(saConn)

	c, err := makeClient()
	parts := strings.SplitAfter(c.URL, "/")
	regID, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	domain := random_domain()
	serverIdents := identifier.ACMEIdentifiers{identifier.NewDNS(domain)}
	clientIdents := []acme.Identifier{{Type: "dns", Value: domain}}

	_, err = saClient.PauseIdentifiers(context.Background(), &sapb.PauseRequest{
		RegistrationID: regID,
		Identifiers:    serverIdents.ToProtoSlice(),
	})
	test.AssertNotError(t, err, "Failed to pause domain")

	_, err = authAndIssue(c, nil, clientIdents, true, "")
	test.AssertError(t, err, "Should not be able to issue a certificate for a paused domain")
	test.AssertContains(t, err.Error(), "Your account is temporarily prevented from requesting certificates for")
	test.AssertContains(t, err.Error(), "https://boulder.service.consul:4003/sfe/v1/unpause?jwt=")

	_, err = saClient.UnpauseAccount(context.Background(), &sapb.RegistrationID{
		Id: regID,
	})
	test.AssertNotError(t, err, "Failed to unpause domain")

	_, err = authAndIssue(c, nil, clientIdents, true, "")
	test.AssertNotError(t, err, "Should be able to issue a certificate for an unpaused domain")
}
