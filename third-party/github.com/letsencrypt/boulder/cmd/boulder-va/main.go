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
	"github.com/letsencrypt/boulder/va"
	vaConfig "github.com/letsencrypt/boulder/va/config"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

type Config struct {
	VA struct {
		vaConfig.Common
		RemoteVAs                   []cmd.GRPCClientConfig `validate:"omitempty,dive"`
		MaxRemoteValidationFailures int                    `validate:"omitempty,min=0,required_with=RemoteVAs"`
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
	proto := "udp"
	if features.Get().DOH {
		proto = "tcp"
	}

	if len(c.VA.DNSStaticResolvers) != 0 {
		servers, err = bdns.NewStaticProvider(c.VA.DNSStaticResolvers)
		cmd.FailOnError(err, "Couldn't start static DNS server resolver")
	} else {
		servers, err = bdns.StartDynamicProvider(c.VA.DNSProvider, 60*time.Second, proto)
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
			logger,
			tlsConfig)
	} else {
		resolver = bdns.NewTest(
			c.VA.DNSTimeout.Duration,
			servers,
			scope,
			clk,
			c.VA.DNSTries,
			logger,
			tlsConfig)
	}
	var remotes []va.RemoteVA
	if len(c.VA.RemoteVAs) > 0 {
		for _, rva := range c.VA.RemoteVAs {
			rva := rva
			vaConn, err := bgrpc.ClientSetup(&rva, tlsConfig, scope, clk)
			cmd.FailOnError(err, "Unable to create remote VA client")
			remotes = append(
				remotes,
				va.RemoteVA{
					RemoteClients: va.RemoteClients{
						VAClient:  vapb.NewVAClient(vaConn),
						CAAClient: vapb.NewCAAClient(vaConn),
					},
					Address: rva.ServerAddress,
				},
			)
		}
	}

	vai, err := va.NewValidationAuthorityImpl(
		resolver,
		remotes,
		c.VA.MaxRemoteValidationFailures,
		c.VA.UserAgent,
		c.VA.IssuerDomain,
		scope,
		clk,
		logger,
		c.VA.AccountURIPrefixes)
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
