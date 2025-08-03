package notmain

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	ct "github.com/google/certificate-transparency-go"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/issuance"
	"github.com/letsencrypt/boulder/publisher"
	pubpb "github.com/letsencrypt/boulder/publisher/proto"
)

type Config struct {
	Publisher struct {
		cmd.ServiceConfig
		Features features.Config

		// If this is non-zero, profile blocking events such that one even is
		// sampled every N nanoseconds.
		// https://golang.org/pkg/runtime/#SetBlockProfileRate
		BlockProfileRate int
		UserAgent        string

		// Chains is a list of lists of certificate filenames. Each inner list is
		// a chain, starting with the issuing intermediate, followed by one or
		// more additional certificates, up to and including a root.
		Chains [][]string `validate:"min=1,dive,min=2,dive,required"`
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
	features.Set(c.Publisher.Features)

	runtime.SetBlockProfileRate(c.Publisher.BlockProfileRate)

	if *grpcAddr != "" {
		c.Publisher.GRPC.Address = *grpcAddr
	}
	if *debugAddr != "" {
		c.Publisher.DebugAddr = *debugAddr
	}
	if c.Publisher.UserAgent == "" {
		c.Publisher.UserAgent = "certificate-transparency-go/1.0"
	}
	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.Publisher.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())

	if c.Publisher.Chains == nil {
		logger.AuditErr("No chain files provided")
		os.Exit(1)
	}

	bundles := make(map[issuance.NameID][]ct.ASN1Cert)
	for _, files := range c.Publisher.Chains {
		chain, err := issuance.LoadChain(files)
		cmd.FailOnError(err, "failed to load chain.")
		issuer := chain[0]
		id := issuer.NameID()
		if _, exists := bundles[id]; exists {
			cmd.Fail(fmt.Sprintf("Got multiple chains configured for issuer %q", issuer.Subject.CommonName))
		}
		bundles[id] = publisher.GetCTBundleForChain(chain)
	}

	tlsConfig, err := c.Publisher.TLS.Load(scope)
	cmd.FailOnError(err, "TLS config")

	clk := cmd.Clock()

	pubi := publisher.New(bundles, c.Publisher.UserAgent, logger, scope)

	start, err := bgrpc.NewServer(c.Publisher.GRPC, logger).Add(
		&pubpb.Publisher_ServiceDesc, pubi).Build(tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to setup Publisher gRPC server")

	cmd.FailOnError(start(), "Publisher gRPC service failed")
}

func init() {
	cmd.RegisterCommand("boulder-publisher", main, &cmd.ConfigValidator{Config: &Config{}})
}
