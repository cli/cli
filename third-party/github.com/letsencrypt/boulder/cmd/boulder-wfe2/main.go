package notmain

import (
	"bytes"
	"context"
	"encoding/pem"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	emailpb "github.com/letsencrypt/boulder/email/proto"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/goodkey/sagoodkey"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/grpc/noncebalancer"
	"github.com/letsencrypt/boulder/issuance"
	"github.com/letsencrypt/boulder/nonce"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/ratelimits"
	bredis "github.com/letsencrypt/boulder/redis"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/unpause"
	"github.com/letsencrypt/boulder/web"
	"github.com/letsencrypt/boulder/wfe2"
)

type Config struct {
	WFE struct {
		DebugAddr string `validate:"omitempty,hostname_port"`

		// ListenAddress is the address:port on which to listen for incoming
		// HTTP requests. Defaults to ":80".
		ListenAddress string `validate:"omitempty,hostname_port"`

		// TLSListenAddress is the address:port on which to listen for incoming
		// HTTPS requests. If none is provided the WFE will not listen for HTTPS
		// requests.
		TLSListenAddress string `validate:"omitempty,hostname_port"`

		// Timeout is the per-request overall timeout. This should be slightly
		// lower than the upstream's timeout when making requests to this service.
		Timeout config.Duration `validate:"-"`

		// ShutdownStopTimeout determines the maximum amount of time to wait
		// for extant request handlers to complete before exiting. It should be
		// greater than Timeout.
		ShutdownStopTimeout config.Duration

		ServerCertificatePath string `validate:"required_with=TLSListenAddress"`
		ServerKeyPath         string `validate:"required_with=TLSListenAddress"`

		AllowOrigins []string

		SubscriberAgreementURL string

		TLS cmd.TLSConfig

		RAService     *cmd.GRPCClientConfig
		SAService     *cmd.GRPCClientConfig
		EmailExporter *cmd.GRPCClientConfig

		// GetNonceService is a gRPC config which contains a single SRV name
		// used to lookup nonce-service instances used exclusively for nonce
		// creation. In a multi-DC deployment this should refer to local
		// nonce-service instances only.
		GetNonceService *cmd.GRPCClientConfig `validate:"required"`

		// RedeemNonceService is a gRPC config which contains a list of SRV
		// names used to lookup nonce-service instances used exclusively for
		// nonce redemption. In a multi-DC deployment this should contain both
		// local and remote nonce-service instances.
		RedeemNonceService *cmd.GRPCClientConfig `validate:"required"`

		// NonceHMACKey is a path to a file containing an HMAC key which is a
		// secret used for deriving the prefix of each nonce instance. It should
		// contain 256 bits (32 bytes) of random data to be suitable as an
		// HMAC-SHA256 key (e.g. the output of `openssl rand -hex 32`). In a
		// multi-DC deployment this value should be the same across all
		// boulder-wfe and nonce-service instances.
		NonceHMACKey cmd.HMACKeyConfig `validate:"-"`

		// Chains is a list of lists of certificate filenames. Each inner list is
		// a chain (starting with the issuing intermediate, followed by one or
		// more additional certificates, up to and including a root) which we are
		// willing to serve. Chains that start with a given intermediate will only
		// be offered for certificates which were issued by the key pair represented
		// by that intermediate. The first chain representing any given issuing
		// key pair will be the default for that issuer, served if the client does
		// not request a specific chain.
		Chains [][]string `validate:"required,min=1,dive,min=2,dive,required"`

		Features features.Config

		// DirectoryCAAIdentity is used for the /directory response's "meta"
		// element's "caaIdentities" field. It should match the VA's "issuerDomain"
		// configuration value (this value is the one used to enforce CAA)
		DirectoryCAAIdentity string `validate:"required,fqdn"`
		// DirectoryWebsite is used for the /directory response's "meta" element's
		// "website" field.
		DirectoryWebsite string `validate:"required,url"`

		// ACMEv2 requests (outside some registration/revocation messages) use a JWS with
		// a KeyID header containing the full account URL. For new accounts this
		// will be a KeyID based on the HTTP request's Host header and the ACMEv2
		// account path. For legacy ACMEv1 accounts we need to whitelist the account
		// ID prefix that legacy accounts would have been using based on the Host
		// header of the WFE1 instance and the legacy 'reg' path component. This
		// will differ in configuration for production and staging.
		LegacyKeyIDPrefix string `validate:"required,url"`

		// GoodKey is an embedded config stanza for the goodkey library.
		GoodKey goodkey.Config

		// StaleTimeout determines how old should data be to be accessed via Boulder-specific GET-able APIs
		StaleTimeout config.Duration `validate:"-"`

		// AuthorizationLifetimeDays duplicates the RA's config of the same name.
		// Deprecated: This field no longer has any effect.
		AuthorizationLifetimeDays int `validate:"-"`

		// PendingAuthorizationLifetimeDays duplicates the RA's config of the same name.
		// Deprecated: This field no longer has any effect.
		PendingAuthorizationLifetimeDays int `validate:"-"`

		// MaxContactsPerRegistration limits the number of contact addresses which
		// can be provided in a single NewAccount request. Requests containing more
		// contacts than this are rejected. Default: 10.
		MaxContactsPerRegistration int `validate:"omitempty,min=1"`

		AccountCache *CacheConfig

		Limiter struct {
			// Redis contains the configuration necessary to connect to Redis
			// for rate limiting. This field is required to enable rate
			// limiting.
			Redis *bredis.Config `validate:"required_with=Defaults"`

			// Defaults is a path to a YAML file containing default rate limits.
			// See: ratelimits/README.md for details. This field is required to
			// enable rate limiting. If any individual rate limit is not set,
			// that limit will be disabled. Failed Authorizations limits passed
			// in this file must be identical to those in the RA.
			Defaults string `validate:"required_with=Redis"`

			// Overrides is a path to a YAML file containing overrides for the
			// default rate limits. See: ratelimits/README.md for details. If
			// this field is not set, all requesters will be subject to the
			// default rate limits. Overrides for the Failed Authorizations
			// overrides passed in this file must be identical to those in the
			// RA.
			Overrides string
		}

		// CertProfiles is a map of acceptable certificate profile names to
		// descriptions (perhaps including URLs) of those profiles. NewOrder
		// Requests with a profile name not present in this map will be rejected.
		// This field is optional; if unset, no profile names are accepted.
		CertProfiles map[string]string `validate:"omitempty,dive,keys,alphanum,min=1,max=32,endkeys"`

		Unpause struct {
			// HMACKey signs outgoing JWTs for redemption at the unpause
			// endpoint. This key must match the one configured for all SFEs.
			// This field is required to enable the pausing feature.
			HMACKey cmd.HMACKeyConfig `validate:"required_with=JWTLifetime URL,structonly"`

			// JWTLifetime is the lifetime of the unpause JWTs generated by the
			// WFE for redemption at the SFE. The minimum value for this field
			// is 336h (14 days). This field is required to enable the pausing
			// feature.
			JWTLifetime config.Duration `validate:"omitempty,required_with=HMACKey URL,min=336h"`

			// URL is the URL of the Self-Service Frontend (SFE). This is used
			// to build URLs sent to end-users in error messages. This field
			// must be a URL with a scheme of 'https://' This field is required
			// to enable the pausing feature.
			URL string `validate:"omitempty,required_with=HMACKey JWTLifetime,url,startswith=https://,endsnotwith=/"`
		}
	}

	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig

	// OpenTelemetryHTTPConfig configures tracing on incoming HTTP requests
	OpenTelemetryHTTPConfig cmd.OpenTelemetryHTTPConfig
}

type CacheConfig struct {
	Size int
	TTL  config.Duration
}

// loadChain takes a list of filenames containing pem-formatted certificates,
// and returns a chain representing all of those certificates in order. It
// ensures that the resulting chain is valid. The final file is expected to be
// a root certificate, which the chain will be verified against, but which will
// not be included in the resulting chain.
func loadChain(certFiles []string) (*issuance.Certificate, []byte, error) {
	certs, err := issuance.LoadChain(certFiles)
	if err != nil {
		return nil, nil, err
	}

	// Iterate over all certs appending their pem to the buf.
	var buf bytes.Buffer
	for _, cert := range certs {
		buf.Write([]byte("\n"))
		buf.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
	}

	return certs[0], buf.Bytes(), nil
}

func main() {
	listenAddr := flag.String("addr", "", "HTTP listen address override")
	tlsAddr := flag.String("tls-addr", "", "HTTPS listen address override")
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

	features.Set(c.WFE.Features)

	if *listenAddr != "" {
		c.WFE.ListenAddress = *listenAddr
	}
	if *tlsAddr != "" {
		c.WFE.TLSListenAddress = *tlsAddr
	}
	if *debugAddr != "" {
		c.WFE.DebugAddr = *debugAddr
	}

	certChains := map[issuance.NameID][][]byte{}
	issuerCerts := map[issuance.NameID]*issuance.Certificate{}
	for _, files := range c.WFE.Chains {
		issuer, chain, err := loadChain(files)
		cmd.FailOnError(err, "Failed to load chain")

		id := issuer.NameID()
		certChains[id] = append(certChains[id], chain)
		// This may overwrite a previously-set issuerCert (e.g. if there are two
		// chains for the same issuer, but with different versions of the same
		// same intermediate issued by different roots). This is okay, as the
		// only truly important content here is the public key to verify other
		// certs.
		issuerCerts[id] = issuer
	}

	stats, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.WFE.DebugAddr)
	logger.Info(cmd.VersionString())

	clk := cmd.Clock()

	var unpauseSigner unpause.JWTSigner
	if features.Get().CheckIdentifiersPaused {
		unpauseSigner, err = unpause.NewJWTSigner(c.WFE.Unpause.HMACKey)
		cmd.FailOnError(err, "Failed to create unpause signer from HMACKey")
	}

	tlsConfig, err := c.WFE.TLS.Load(stats)
	cmd.FailOnError(err, "TLS config")

	raConn, err := bgrpc.ClientSetup(c.WFE.RAService, tlsConfig, stats, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to RA")
	rac := rapb.NewRegistrationAuthorityClient(raConn)

	saConn, err := bgrpc.ClientSetup(c.WFE.SAService, tlsConfig, stats, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to SA")
	sac := sapb.NewStorageAuthorityReadOnlyClient(saConn)

	var eec emailpb.ExporterClient
	if c.WFE.EmailExporter != nil {
		emailExporterConn, err := bgrpc.ClientSetup(c.WFE.EmailExporter, tlsConfig, stats, clk)
		cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to email-exporter")
		eec = emailpb.NewExporterClient(emailExporterConn)
	}

	if c.WFE.RedeemNonceService == nil {
		cmd.Fail("'redeemNonceService' must be configured.")
	}
	if c.WFE.GetNonceService == nil {
		cmd.Fail("'getNonceService' must be configured")
	}

	noncePrefixKey, err := c.WFE.NonceHMACKey.Load()
	cmd.FailOnError(err, "Failed to load nonceHMACKey file")

	getNonceConn, err := bgrpc.ClientSetup(c.WFE.GetNonceService, tlsConfig, stats, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to get nonce service")
	gnc := nonce.NewGetter(getNonceConn)

	if c.WFE.RedeemNonceService.SRVResolver != noncebalancer.SRVResolverScheme {
		cmd.Fail(fmt.Sprintf(
			"'redeemNonceService.SRVResolver' must be set to %q", noncebalancer.SRVResolverScheme),
		)
	}
	redeemNonceConn, err := bgrpc.ClientSetup(c.WFE.RedeemNonceService, tlsConfig, stats, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to redeem nonce service")
	rnc := nonce.NewRedeemer(redeemNonceConn)

	kp, err := sagoodkey.NewPolicy(&c.WFE.GoodKey, sac.KeyBlocked)
	cmd.FailOnError(err, "Unable to create key policy")

	if c.WFE.StaleTimeout.Duration == 0 {
		c.WFE.StaleTimeout.Duration = time.Minute * 10
	}

	if c.WFE.MaxContactsPerRegistration == 0 {
		c.WFE.MaxContactsPerRegistration = 10
	}

	var limiter *ratelimits.Limiter
	var txnBuilder *ratelimits.TransactionBuilder
	var limiterRedis *bredis.Ring
	if c.WFE.Limiter.Defaults != "" {
		// Setup rate limiting.
		limiterRedis, err = bredis.NewRingFromConfig(*c.WFE.Limiter.Redis, stats, logger)
		cmd.FailOnError(err, "Failed to create Redis ring")

		source := ratelimits.NewRedisSource(limiterRedis.Ring, clk, stats)
		limiter, err = ratelimits.NewLimiter(clk, source, stats)
		cmd.FailOnError(err, "Failed to create rate limiter")
		txnBuilder, err = ratelimits.NewTransactionBuilderFromFiles(c.WFE.Limiter.Defaults, c.WFE.Limiter.Overrides)
		cmd.FailOnError(err, "Failed to create rate limits transaction builder")
	}

	var accountGetter wfe2.AccountGetter
	if c.WFE.AccountCache != nil {
		accountGetter = wfe2.NewAccountCache(sac,
			c.WFE.AccountCache.Size,
			c.WFE.AccountCache.TTL.Duration,
			clk,
			stats)
	} else {
		accountGetter = sac
	}
	wfe, err := wfe2.NewWebFrontEndImpl(
		stats,
		clk,
		kp,
		certChains,
		issuerCerts,
		logger,
		c.WFE.Timeout.Duration,
		c.WFE.StaleTimeout.Duration,
		c.WFE.MaxContactsPerRegistration,
		rac,
		sac,
		eec,
		gnc,
		rnc,
		noncePrefixKey,
		accountGetter,
		limiter,
		txnBuilder,
		c.WFE.CertProfiles,
		unpauseSigner,
		c.WFE.Unpause.JWTLifetime.Duration,
		c.WFE.Unpause.URL,
	)
	cmd.FailOnError(err, "Unable to create WFE")

	wfe.SubscriberAgreementURL = c.WFE.SubscriberAgreementURL
	wfe.AllowOrigins = c.WFE.AllowOrigins
	wfe.DirectoryCAAIdentity = c.WFE.DirectoryCAAIdentity
	wfe.DirectoryWebsite = c.WFE.DirectoryWebsite
	wfe.LegacyKeyIDPrefix = c.WFE.LegacyKeyIDPrefix

	logger.Infof("WFE using key policy: %#v", kp)

	if c.WFE.ListenAddress == "" {
		cmd.Fail("HTTP listen address is not configured")
	}

	logger.Infof("Server running, listening on %s....", c.WFE.ListenAddress)
	handler := wfe.Handler(stats, c.OpenTelemetryHTTPConfig.Options()...)

	srv := web.NewServer(c.WFE.ListenAddress, handler, logger)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			cmd.FailOnError(err, "Running HTTP server")
		}
	}()

	tlsSrv := web.NewServer(c.WFE.TLSListenAddress, handler, logger)
	if tlsSrv.Addr != "" {
		go func() {
			logger.Infof("TLS server listening on %s", tlsSrv.Addr)
			err := tlsSrv.ListenAndServeTLS(c.WFE.ServerCertificatePath, c.WFE.ServerKeyPath)
			if err != nil && err != http.ErrServerClosed {
				cmd.FailOnError(err, "Running TLS server")
			}
		}()
	}

	// When main is ready to exit (because it has received a shutdown signal),
	// gracefully shutdown the servers. Calling these shutdown functions causes
	// ListenAndServe() and ListenAndServeTLS() to immediately return, then waits
	// for any lingering connection-handling goroutines to finish their work.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), c.WFE.ShutdownStopTimeout.Duration)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = tlsSrv.Shutdown(ctx)
		limiterRedis.StopLookups()
		oTelShutdown(ctx)
	}()

	cmd.WaitForSignal()
}

func init() {
	cmd.RegisterCommand("boulder-wfe2", main, &cmd.ConfigValidator{Config: &Config{}})
}
