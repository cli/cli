package notmain

import (
	"context"
	"crypto/tls"
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
	RVA struct {
		vaConfig.Common

		// SkipGRPCClientCertVerification, when disabled as it should typically
		// be, will cause the remoteva server (which receives gRPCs from a
		// boulder-va client) to use our default RequireAndVerifyClientCert
		// policy. When enabled, the remoteva server will instead use the less
		// secure VerifyClientCertIfGiven policy. It should typically be used in
		// conjunction with the boulder-va "RVATLSClient" configuration object.
		//
		// An operator may choose to enable this if the remoteva server is
		// logically behind an OSI layer-7 loadbalancer/reverse proxy which
		// decrypts traffic and does not/cannot re-encrypt it's own client
		// connection to the remoteva server.
		//
		// Use with caution.
		//
		// For more information, see: https://pkg.go.dev/crypto/tls#ClientAuthType
		SkipGRPCClientCertVerification bool

		Features features.Config
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
	err = c.RVA.SetDefaultsAndValidate(grpcAddr, debugAddr)
	cmd.FailOnError(err, "Setting and validating default config values")
	features.Set(c.RVA.Features)

	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.RVA.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())
	clk := cmd.Clock()

	var servers bdns.ServerProvider
	proto := "udp"
	if features.Get().DOH {
		proto = "tcp"
	}

	if len(c.RVA.DNSStaticResolvers) != 0 {
		servers, err = bdns.NewStaticProvider(c.RVA.DNSStaticResolvers)
		cmd.FailOnError(err, "Couldn't start static DNS server resolver")
	} else {
		servers, err = bdns.StartDynamicProvider(c.RVA.DNSProvider, 60*time.Second, proto)
		cmd.FailOnError(err, "Couldn't start dynamic DNS server resolver")
	}
	defer servers.Stop()

	tlsConfig, err := c.RVA.TLS.Load(scope)
	cmd.FailOnError(err, "tlsConfig config")

	if c.RVA.SkipGRPCClientCertVerification {
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	}

	var resolver bdns.Client
	if !c.RVA.DNSAllowLoopbackAddresses {
		resolver = bdns.New(
			c.RVA.DNSTimeout.Duration,
			servers,
			scope,
			clk,
			c.RVA.DNSTries,
			logger,
			tlsConfig)
	} else {
		resolver = bdns.NewTest(
			c.RVA.DNSTimeout.Duration,
			servers,
			scope,
			clk,
			c.RVA.DNSTries,
			logger,
			tlsConfig)
	}

	vai, err := va.NewValidationAuthorityImpl(
		resolver,
		nil, // Our RVAs will never have RVAs of their own.
		0,   // Only the VA is concerned with max validation failures
		c.RVA.UserAgent,
		c.RVA.IssuerDomain,
		scope,
		clk,
		logger,
		c.RVA.AccountURIPrefixes)
	cmd.FailOnError(err, "Unable to create Remote-VA server")

	start, err := bgrpc.NewServer(c.RVA.GRPC, logger).Add(
		&vapb.VA_ServiceDesc, vai).Add(
		&vapb.CAA_ServiceDesc, vai).Build(tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to setup Remote-VA gRPC server")
	cmd.FailOnError(start(), "Remote-VA gRPC service failed")
}

func init() {
	cmd.RegisterCommand("remoteva", main, &cmd.ConfigValidator{Config: &Config{}})
}
