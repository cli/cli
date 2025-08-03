package notmain

import (
	"context"
	"flag"
	"net/http"
	"os"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/sfe"
	"github.com/letsencrypt/boulder/web"
)

type Config struct {
	SFE struct {
		DebugAddr string `validate:"omitempty,hostname_port"`

		// ListenAddress is the address:port on which to listen for incoming
		// HTTP requests. Defaults to ":80".
		ListenAddress string `validate:"omitempty,hostname_port"`

		// Timeout is the per-request overall timeout. This should be slightly
		// lower than the upstream's timeout when making requests to this service.
		Timeout config.Duration `validate:"-"`

		// ShutdownStopTimeout determines the maximum amount of time to wait
		// for extant request handlers to complete before exiting. It should be
		// greater than Timeout.
		ShutdownStopTimeout config.Duration

		TLS cmd.TLSConfig

		RAService *cmd.GRPCClientConfig
		SAService *cmd.GRPCClientConfig

		// UnpauseHMACKey validates incoming JWT signatures at the unpause
		// endpoint. This key must be the same as the one configured for all
		// WFEs. This field is required to enable the pausing feature.
		UnpauseHMACKey cmd.HMACKeyConfig

		Features features.Config
	}

	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig

	// OpenTelemetryHTTPConfig configures tracing on incoming HTTP requests
	OpenTelemetryHTTPConfig cmd.OpenTelemetryHTTPConfig
}

func main() {
	listenAddr := flag.String("addr", "", "HTTP listen address override")
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

	features.Set(c.SFE.Features)

	if *listenAddr != "" {
		c.SFE.ListenAddress = *listenAddr
	}
	if c.SFE.ListenAddress == "" {
		cmd.Fail("HTTP listen address is not configured")
	}
	if *debugAddr != "" {
		c.SFE.DebugAddr = *debugAddr
	}

	stats, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.SFE.DebugAddr)
	logger.Info(cmd.VersionString())

	clk := cmd.Clock()

	unpauseHMACKey, err := c.SFE.UnpauseHMACKey.Load()
	cmd.FailOnError(err, "Failed to load unpauseHMACKey")

	tlsConfig, err := c.SFE.TLS.Load(stats)
	cmd.FailOnError(err, "TLS config")

	raConn, err := bgrpc.ClientSetup(c.SFE.RAService, tlsConfig, stats, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to RA")
	rac := rapb.NewRegistrationAuthorityClient(raConn)

	saConn, err := bgrpc.ClientSetup(c.SFE.SAService, tlsConfig, stats, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to SA")
	sac := sapb.NewStorageAuthorityReadOnlyClient(saConn)

	sfei, err := sfe.NewSelfServiceFrontEndImpl(
		stats,
		clk,
		logger,
		c.SFE.Timeout.Duration,
		rac,
		sac,
		unpauseHMACKey,
	)
	cmd.FailOnError(err, "Unable to create SFE")

	logger.Infof("Server running, listening on %s....", c.SFE.ListenAddress)
	handler := sfei.Handler(stats, c.OpenTelemetryHTTPConfig.Options()...)

	srv := web.NewServer(c.SFE.ListenAddress, handler, logger)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			cmd.FailOnError(err, "Running HTTP server")
		}
	}()

	// When main is ready to exit (because it has received a shutdown signal),
	// gracefully shutdown the servers. Calling these shutdown functions causes
	// ListenAndServe() and ListenAndServeTLS() to immediately return, then waits
	// for any lingering connection-handling goroutines to finish their work.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), c.SFE.ShutdownStopTimeout.Duration)
		defer cancel()
		_ = srv.Shutdown(ctx)
		oTelShutdown(ctx)
	}()

	cmd.WaitForSignal()
}

func init() {
	cmd.RegisterCommand("sfe", main, &cmd.ConfigValidator{Config: &Config{}})
}
