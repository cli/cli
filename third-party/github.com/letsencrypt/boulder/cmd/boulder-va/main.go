package notmain

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/iana"
	"github.com/letsencrypt/boulder/va"
	vaConfig "github.com/letsencrypt/boulder/va/config"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

// RemoteVAGRPCClientConfig  contains the information necessary to setup a gRPC
// client connection. The following GRPC client configuration field combinations
// are allowed:
//
// ServerIPAddresses, [Timeout]
// ServerAddress, DNSAuthority, [Timeout], [HostOverride]
// SRVLookup, DNSAuthority, [Timeout], [HostOverride], [SRVResolver]
// SRVLookups, DNSAuthority, [Timeout], [HostOverride], [SRVResolver]
type RemoteVAGRPCClientConfig struct {
	cmd.GRPCClientConfig
	// Perspective uniquely identifies the Network Perspective used to
	// perform the validation, as specified in BRs Section 5.4.1,
	// Requirement 2.7 ("Multi-Perspective Issuance Corroboration attempts
	// from each Network Perspective"). It should uniquely identify a group
	// of RVAs deployed in the same datacenter.
	Perspective string `validate:"required"`

	// RIR indicates the Regional Internet Registry where this RVA is
	// located. This field is used to identify the RIR region from which a
	// given validation was performed, as specified in the "Phased
	// Implementation Timeline" in BRs Section 3.2.2.9. It must be one of
	// the following values:
	//   - ARIN
	//   - RIPE
	//   - APNIC
	//   - LACNIC
	//   - AFRINIC
	RIR string `validate:"required,oneof=ARIN RIPE APNIC LACNIC AFRINIC"`
}

type Config struct {
	VA struct {
		vaConfig.Common
		RemoteVAs []RemoteVAGRPCClientConfig `validate:"omitempty,dive"`
		// Deprecated and ignored
		MaxRemoteValidationFailures int `validate:"omitempty,min=0,required_with=RemoteVAs"`
		Features                    features.Config
	}

	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig
}

func main() {
	grpcAddr := flag.String("addr", "", "gRPC listen address override")
	debugAddr := flag.String("debug-addr", "", "Debug server address override")
	configFile := flag.String("config", "", "File path to the configuration file for this service")
	flag.Parse()
	if *configFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	var c Config
	err := cmd.ReadConfigFile(*configFile, &c)
	cmd.FailOnError(err, "Reading JSON config file into config structure")
	err = c.VA.SetDefaultsAndValidate(grpcAddr, debugAddr)
	cmd.FailOnError(err, "Setting and validating default config values")

	features.Set(c.VA.Features)
	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.VA.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())
	clk := cmd.Clock()

	var servers bdns.ServerProvider

	if len(c.VA.DNSStaticResolvers) != 0 {
		servers, err = bdns.NewStaticProvider(c.VA.DNSStaticResolvers)
		cmd.FailOnError(err, "Couldn't start static DNS server resolver")
	} else {
		servers, err = bdns.StartDynamicProvider(c.VA.DNSProvider, 60*time.Second, "tcp")
		cmd.FailOnError(err, "Couldn't start dynamic DNS server resolver")
	}
	defer servers.Stop()

	tlsConfig, err := c.VA.TLS.Load(scope)
	cmd.FailOnError(err, "tlsConfig config")

	var resolver bdns.Client
	if !c.VA.DNSAllowLoopbackAddresses {
		resolver = bdns.New(
			c.VA.DNSTimeout.Duration,
			servers,
			scope,
			clk,
			c.VA.DNSTries,
			c.VA.UserAgent,
			logger,
			tlsConfig)
	} else {
		resolver = bdns.NewTest(
			c.VA.DNSTimeout.Duration,
			servers,
			scope,
			clk,
			c.VA.DNSTries,
			c.VA.UserAgent,
			logger,
			tlsConfig)
	}
	var remotes []va.RemoteVA
	if len(c.VA.RemoteVAs) > 0 {
		for _, rva := range c.VA.RemoteVAs {
			rva := rva
			vaConn, err := bgrpc.ClientSetup(&rva.GRPCClientConfig, tlsConfig, scope, clk)
			cmd.FailOnError(err, "Unable to create remote VA client")
			remotes = append(
				remotes,
				va.RemoteVA{
					RemoteClients: va.RemoteClients{
						VAClient:  vapb.NewVAClient(vaConn),
						CAAClient: vapb.NewCAAClient(vaConn),
					},
					Address:     rva.ServerAddress,
					Perspective: rva.Perspective,
					RIR:         rva.RIR,
				},
			)
		}
	}

	vai, err := va.NewValidationAuthorityImpl(
		resolver,
		remotes,
		c.VA.UserAgent,
		c.VA.IssuerDomain,
		scope,
		clk,
		logger,
		c.VA.AccountURIPrefixes,
		va.PrimaryPerspective,
		"",
		iana.IsReservedAddr)
	cmd.FailOnError(err, "Unable to create VA server")

	start, err := bgrpc.NewServer(c.VA.GRPC, logger).Add(
		&vapb.VA_ServiceDesc, vai).Add(
		&vapb.CAA_ServiceDesc, vai).Build(tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to setup VA gRPC server")
	cmd.FailOnError(start(), "VA gRPC service failed")
}

func init() {
	cmd.RegisterCommand("boulder-va", main, &cmd.ConfigValidator{Config: &Config{}})
}
