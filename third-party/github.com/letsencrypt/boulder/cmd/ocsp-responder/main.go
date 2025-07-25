package notmain

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/db"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics/measured_http"
	"github.com/letsencrypt/boulder/ocsp/responder"
	"github.com/letsencrypt/boulder/ocsp/responder/live"
	redis_responder "github.com/letsencrypt/boulder/ocsp/responder/redis"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	rocsp_config "github.com/letsencrypt/boulder/rocsp/config"
	"github.com/letsencrypt/boulder/sa"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

type Config struct {
	OCSPResponder struct {
		DebugAddr string       `validate:"omitempty,hostname_port"`
		DB        cmd.DBConfig `validate:"required_without_all=Source SAService,structonly"`

		// Source indicates the source of pre-signed OCSP responses to be used. It
		// can be a DBConnect string or a file URL. The file URL style is used
		// when responding from a static file for intermediates and roots.
		// If DBConfig has non-empty fields, it takes precedence over this.
		Source string `validate:"required_without_all=DB.DBConnectFile SAService Redis"`

		// The list of issuer certificates, against which OCSP requests/responses
		// are checked to ensure we're not responding for anyone else's certs.
		IssuerCerts []string `validate:"min=1,dive,required"`

		Path string

		// ListenAddress is the address:port on which to listen for incoming
		// OCSP requests. This has a default value of ":80".
		ListenAddress string `validate:"omitempty,hostname_port"`

		// Timeout is the per-request overall timeout. This should be slightly
		// lower than the upstream's timeout when making requests to this service.
		Timeout config.Duration `validate:"-"`

		// ShutdownStopTimeout determines the maximum amount of time to wait
		// for extant request handlers to complete before exiting. It should be
		// greater than Timeout.
		ShutdownStopTimeout config.Duration

		// How often a response should be signed when using Redis/live-signing
		// path. This has a default value of 60h.
		LiveSigningPeriod config.Duration `validate:"-"`

		// A limit on how many requests to the RA (and onwards to the CA) will
		// be made to sign responses that are not fresh in the cache. This
		// should be set to somewhat less than
		// (HSM signing capacity) / (number of ocsp-responders).
		// Requests that would exceed this limit will block until capacity is
		// available and eventually serve an HTTP 500 Internal Server Error.
		// This has a default value of 1000.
		MaxInflightSignings int `validate:"min=0"`

		// A limit on how many goroutines can be waiting for a signing slot at
		// a time. When this limit is exceeded, additional signing requests
		// will immediately serve an HTTP 500 Internal Server Error until
		// we are back below the limit. This provides load shedding for when
		// inbound requests arrive faster than our ability to sign them.
		// The default of 0 means "no limit." A good value for this is the
		// longest queue we can expect to process before a timeout. For
		// instance, if the timeout is 5 seconds, and a signing takes 20ms,
		// and we have MaxInflightSignings = 40, we can expect to process
		// 40 * 5 / 0.02 = 10,000 requests before the oldest request times out.
		MaxSigningWaiters int `validate:"min=0"`

		RequiredSerialPrefixes []string `validate:"omitempty,dive,hexadecimal"`

		Features features.Config

		// Configuration for using Redis as a cache. This configuration should
		// allow for both read and write access.
		Redis *rocsp_config.RedisConfig `validate:"required_without=Source"`

		// TLS client certificate, private key, and trusted root bundle.
		TLS cmd.TLSConfig `validate:"required_without=Source,structonly"`

		// RAService configures how to communicate with the RA when it is necessary
		// to generate a fresh OCSP response.
		RAService *cmd.GRPCClientConfig

		// SAService configures how to communicate with the SA to look up
		// certificate status metadata used to confirm/deny that the response from
		// Redis is up-to-date.
		SAService *cmd.GRPCClientConfig `validate:"required_without_all=DB.DBConnectFile Source"`

		// LogSampleRate sets how frequently error logs should be emitted. This
		// avoids flooding the logs during outages. 1 out of N log lines will be emitted.
		// If LogSampleRate is 0, no logs will be emitted.
		LogSampleRate int `validate:"min=0"`
	}

	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig

	// OpenTelemetryHTTPConfig configures tracing on incoming HTTP requests
	OpenTelemetryHTTPConfig cmd.OpenTelemetryHTTPConfig
}

func main() {
	listenAddr := flag.String("addr", "", "OCSP listen address override")
	debugAddr := flag.String("debug-addr", "", "Debug server address override")
	configFile := flag.String("config", "", "File path to the configuration file for this service")
	flag.Parse()

	if *configFile == "" {
		fmt.Fprintf(os.Stderr, `Usage of %s:
Config JSON should contain either a DBConnectFile or a Source value containing a file: URL.
If Source is a file: URL, the file should contain a list of OCSP responses in base64-encoded DER,
as generated by Boulder's ceremony command.
`, os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	var c Config
	err := cmd.ReadConfigFile(*configFile, &c)
	cmd.FailOnError(err, "Reading JSON config file into config structure")

	features.Set(c.OCSPResponder.Features)

	if *listenAddr != "" {
		c.OCSPResponder.ListenAddress = *listenAddr
	}
	if *debugAddr != "" {
		c.OCSPResponder.DebugAddr = *debugAddr
	}

	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.OCSPResponder.DebugAddr)
	logger.Info(cmd.VersionString())

	clk := cmd.Clock()

	var source responder.Source

	if strings.HasPrefix(c.OCSPResponder.Source, "file:") {
		url, err := url.Parse(c.OCSPResponder.Source)
		cmd.FailOnError(err, "Source was not a URL")
		filename := url.Path
		// Go interprets cwd-relative file urls (file:test/foo.txt) as having the
		// relative part of the path in the 'Opaque' field.
		if filename == "" {
			filename = url.Opaque
		}
		source, err = responder.NewMemorySourceFromFile(filename, logger)
		cmd.FailOnError(err, fmt.Sprintf("Couldn't read file: %s", url.Path))
	} else {
		// Set up the redis source and the combined multiplex source.
		rocspRWClient, err := rocsp_config.MakeClient(c.OCSPResponder.Redis, clk, scope)
		cmd.FailOnError(err, "Could not make redis client")

		err = rocspRWClient.Ping(context.Background())
		cmd.FailOnError(err, "pinging Redis")

		liveSigningPeriod := c.OCSPResponder.LiveSigningPeriod.Duration
		if liveSigningPeriod == 0 {
			liveSigningPeriod = 60 * time.Hour
		}

		tlsConfig, err := c.OCSPResponder.TLS.Load(scope)
		cmd.FailOnError(err, "TLS config")

		raConn, err := bgrpc.ClientSetup(c.OCSPResponder.RAService, tlsConfig, scope, clk)
		cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to RA")
		rac := rapb.NewRegistrationAuthorityClient(raConn)

		maxInflight := c.OCSPResponder.MaxInflightSignings
		if maxInflight == 0 {
			maxInflight = 1000
		}
		liveSource := live.New(rac, int64(maxInflight), c.OCSPResponder.MaxSigningWaiters)

		rocspSource, err := redis_responder.NewRedisSource(rocspRWClient, liveSource, liveSigningPeriod, clk, scope, logger, c.OCSPResponder.LogSampleRate)
		cmd.FailOnError(err, "Could not create redis source")

		var dbMap *db.WrappedMap
		if c.OCSPResponder.DB != (cmd.DBConfig{}) {
			dbMap, err = sa.InitWrappedDb(c.OCSPResponder.DB, scope, logger)
			cmd.FailOnError(err, "While initializing dbMap")
		}

		var sac sapb.StorageAuthorityReadOnlyClient
		if c.OCSPResponder.SAService != nil {
			saConn, err := bgrpc.ClientSetup(c.OCSPResponder.SAService, tlsConfig, scope, clk)
			cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to SA")
			sac = sapb.NewStorageAuthorityReadOnlyClient(saConn)
		}

		source, err = redis_responder.NewCheckedRedisSource(rocspSource, dbMap, sac, scope, logger)
		cmd.FailOnError(err, "Could not create checkedRedis source")
	}

	// Load the certificate from the file path.
	issuerCerts := make([]*issuance.Certificate, len(c.OCSPResponder.IssuerCerts))
	for i, issuerFile := range c.OCSPResponder.IssuerCerts {
		issuerCert, err := issuance.LoadCertificate(issuerFile)
		cmd.FailOnError(err, "Could not load issuer cert")
		issuerCerts[i] = issuerCert
	}

	source, err = responder.NewFilterSource(
		issuerCerts,
		c.OCSPResponder.RequiredSerialPrefixes,
		source,
		scope,
		logger,
		clk,
	)
	cmd.FailOnError(err, "Could not create filtered source")

	m := mux(c.OCSPResponder.Path, source, c.OCSPResponder.Timeout.Duration, scope, c.OpenTelemetryHTTPConfig.Options(), logger, c.OCSPResponder.LogSampleRate)

	if c.OCSPResponder.ListenAddress == "" {
		cmd.Fail("HTTP listen address is not configured")
	}

	logger.Infof("HTTP server listening on %s", c.OCSPResponder.ListenAddress)

	srv := &http.Server{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
		Addr:         c.OCSPResponder.ListenAddress,
		Handler:      m,
	}

	err = srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		cmd.FailOnError(err, "Running HTTP server")
	}

	// When main is ready to exit (because it has received a shutdown signal),
	// gracefully shutdown the servers. Calling these shutdown functions causes
	// ListenAndServe() to immediately return, cleaning up the server goroutines
	// as well, then waits for any lingering connection-handing goroutines to
	// finish and clean themselves up.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(),
			c.OCSPResponder.ShutdownStopTimeout.Duration)
		defer cancel()
		_ = srv.Shutdown(ctx)
		oTelShutdown(ctx)
	}()

	cmd.WaitForSignal()
}

// ocspMux partially implements the interface defined for http.ServeMux but doesn't implement
// the path cleaning its Handler method does. Notably http.ServeMux will collapse repeated
// slashes into a single slash which breaks the base64 encoding that is used in OCSP GET
// requests. ocsp.Responder explicitly recommends against using http.ServeMux
// for this reason.
type ocspMux struct {
	handler http.Handler
}

func (om *ocspMux) Handler(_ *http.Request) (http.Handler, string) {
	return om.handler, "/"
}

func mux(responderPath string, source responder.Source, timeout time.Duration, stats prometheus.Registerer, oTelHTTPOptions []otelhttp.Option, logger blog.Logger, sampleRate int) http.Handler {
	stripPrefix := http.StripPrefix(responderPath, responder.NewResponder(source, timeout, stats, logger, sampleRate))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "max-age=43200") // Cache for 12 hours
			w.WriteHeader(200)
			return
		}
		stripPrefix.ServeHTTP(w, r)
	})
	return measured_http.New(&ocspMux{h}, cmd.Clock(), stats, oTelHTTPOptions...)
}

func init() {
	cmd.RegisterCommand("ocsp-responder", main, &cmd.ConfigValidator{Config: &Config{}})
}
