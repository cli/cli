package notmain

import (
	"context"
	"flag"
	"os"
	"time"

	akamaipb "github.com/letsencrypt/boulder/akamai/proto"
	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/ctpolicy"
	"github.com/letsencrypt/boulder/ctpolicy/ctconfig"
	"github.com/letsencrypt/boulder/ctpolicy/loglist"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/goodkey/sagoodkey"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/issuance"
	"github.com/letsencrypt/boulder/policy"
	pubpb "github.com/letsencrypt/boulder/publisher/proto"
	"github.com/letsencrypt/boulder/ra"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/ratelimits"
	bredis "github.com/letsencrypt/boulder/redis"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

type Config struct {
	RA struct {
		cmd.ServiceConfig
		cmd.HostnamePolicyConfig

		RateLimitPoliciesFilename string `validate:"required"`

		MaxContactsPerRegistration int

		SAService           *cmd.GRPCClientConfig
		VAService           *cmd.GRPCClientConfig
		CAService           *cmd.GRPCClientConfig
		OCSPService         *cmd.GRPCClientConfig
		PublisherService    *cmd.GRPCClientConfig
		AkamaiPurgerService *cmd.GRPCClientConfig

		Limiter struct {
			// Redis contains the configuration necessary to connect to Redis
			// for rate limiting. This field is required to enable rate
			// limiting.
			Redis *bredis.Config `validate:"required_with=Defaults"`

			// Defaults is a path to a YAML file containing default rate limits.
			// See: ratelimits/README.md for details. This field is required to
			// enable rate limiting. If any individual rate limit is not set,
			// that limit will be disabled. Limits passed in this file must be
			// identical to those in the WFE.
			//
			// Note: At this time, only the Failed Authorizations rate limit is
			// necessary in the RA.
			Defaults string `validate:"required_with=Redis"`

			// Overrides is a path to a YAML file containing overrides for the
			// default rate limits. See: ratelimits/README.md for details. If
			// this field is not set, all requesters will be subject to the
			// default rate limits. Overrides passed in this file must be
			// identical to those in the WFE.
			//
			// Note: At this time, only the Failed Authorizations overrides are
			// necessary in the RA.
			Overrides string
		}

		// MaxNames is the maximum number of subjectAltNames in a single cert.
		// The value supplied MUST be greater than 0 and no more than 100. These
		// limits are per section 7.1 of our combined CP/CPS, under "DV-SSL
		// Subscriber Certificate". The value must match the CA and WFE
		// configurations.
		MaxNames int `validate:"required,min=1,max=100"`

		// AuthorizationLifetimeDays defines how long authorizations will be
		// considered valid for. Given a value of 300 days when used with a 90-day
		// cert lifetime, this allows creation of certs that will cover a whole
		// year, plus a grace period of a month.
		AuthorizationLifetimeDays int `validate:"required,min=1,max=397"`

		// PendingAuthorizationLifetimeDays defines how long authorizations may be in
		// the pending state. If you can't respond to a challenge this quickly, then
		// you need to request a new challenge.
		PendingAuthorizationLifetimeDays int `validate:"required,min=1,max=29"`

		// GoodKey is an embedded config stanza for the goodkey library.
		GoodKey goodkey.Config

		// OrderLifetime is how far in the future an Order's expiration date should
		// be set when it is first created.
		OrderLifetime config.Duration

		// FinalizeTimeout is how long the RA is willing to wait for the Order
		// finalization process to take. This config parameter only has an effect
		// if the AsyncFinalization feature flag is enabled. Any systems which
		// manage the shutdown of an RA must be willing to wait at least this long
		// after sending the shutdown signal, to allow background goroutines to
		// complete.
		FinalizeTimeout config.Duration `validate:"-"`

		// CTLogs contains groupings of CT logs organized by what organization
		// operates them. When we submit precerts to logs in order to get SCTs, we
		// will submit the cert to one randomly-chosen log from each group, and use
		// the SCTs from the first two groups which reply. This allows us to comply
		// with various CT policies that require (for certs with short lifetimes
		// like ours) two SCTs from logs run by different operators. It also holds
		// a `Stagger` value controlling how long we wait for one operator group
		// to respond before trying a different one.
		CTLogs ctconfig.CTConfig
		// InformationalCTLogs are a set of CT logs we will always submit to
		// but won't ever use the SCTs from. This may be because we want to
		// test them or because they are not yet approved by a browser/root
		// program but we still want our certs to end up there.
		InformationalCTLogs []ctconfig.LogDescription

		// IssuerCerts are paths to all intermediate certificates which may have
		// been used to issue certificates in the last 90 days. These are used to
		// generate OCSP URLs to purge during revocation.
		IssuerCerts []string `validate:"min=1,dive,required"`

		Features features.Config
	}

	PA cmd.PAConfig

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

	features.Set(c.RA.Features)

	if *grpcAddr != "" {
		c.RA.GRPC.Address = *grpcAddr
	}
	if *debugAddr != "" {
		c.RA.DebugAddr = *debugAddr
	}

	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.RA.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())

	// Validate PA config and set defaults if needed
	cmd.FailOnError(c.PA.CheckChallenges(), "Invalid PA configuration")

	pa, err := policy.New(c.PA.Challenges, logger)
	cmd.FailOnError(err, "Couldn't create PA")

	if c.RA.HostnamePolicyFile == "" {
		cmd.Fail("HostnamePolicyFile must be provided.")
	}
	err = pa.LoadHostnamePolicyFile(c.RA.HostnamePolicyFile)
	cmd.FailOnError(err, "Couldn't load hostname policy file")

	tlsConfig, err := c.RA.TLS.Load(scope)
	cmd.FailOnError(err, "TLS config")

	clk := cmd.Clock()

	vaConn, err := bgrpc.ClientSetup(c.RA.VAService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to create VA client")
	vac := vapb.NewVAClient(vaConn)
	caaClient := vapb.NewCAAClient(vaConn)

	caConn, err := bgrpc.ClientSetup(c.RA.CAService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to create CA client")
	cac := capb.NewCertificateAuthorityClient(caConn)

	ocspConn, err := bgrpc.ClientSetup(c.RA.OCSPService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to create CA OCSP client")
	ocspc := capb.NewOCSPGeneratorClient(ocspConn)

	saConn, err := bgrpc.ClientSetup(c.RA.SAService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to SA")
	sac := sapb.NewStorageAuthorityClient(saConn)

	conn, err := bgrpc.ClientSetup(c.RA.PublisherService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to Publisher")
	pubc := pubpb.NewPublisherClient(conn)

	apConn, err := bgrpc.ClientSetup(c.RA.AkamaiPurgerService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to create a Akamai Purger client")
	apc := akamaipb.NewAkamaiPurgerClient(apConn)

	issuerCertPaths := c.RA.IssuerCerts
	issuerCerts := make([]*issuance.Certificate, len(issuerCertPaths))
	for i, issuerCertPath := range issuerCertPaths {
		issuerCerts[i], err = issuance.LoadCertificate(issuerCertPath)
		cmd.FailOnError(err, "Failed to load issuer certificate")
	}

	// Boulder's components assume that there will always be CT logs configured.
	// Issuing a certificate without SCTs embedded is a misissuance event as per
	// our CPS 4.4.2, which declares we will always include at least two SCTs.
	// Exit early if no groups are configured.
	var ctp *ctpolicy.CTPolicy
	if len(c.RA.CTLogs.SCTLogs) <= 0 {
		cmd.Fail("Must configure CTLogs")
	}

	allLogs, err := loglist.New(c.RA.CTLogs.LogListFile)
	cmd.FailOnError(err, "Failed to parse log list")

	sctLogs, err := allLogs.SubsetForPurpose(c.RA.CTLogs.SCTLogs, loglist.Issuance)
	cmd.FailOnError(err, "Failed to load SCT logs")

	infoLogs, err := allLogs.SubsetForPurpose(c.RA.CTLogs.InfoLogs, loglist.Informational)
	cmd.FailOnError(err, "Failed to load informational logs")

	finalLogs, err := allLogs.SubsetForPurpose(c.RA.CTLogs.FinalLogs, loglist.Informational)
	cmd.FailOnError(err, "Failed to load final logs")

	ctp = ctpolicy.New(pubc, sctLogs, infoLogs, finalLogs, c.RA.CTLogs.Stagger.Duration, logger, scope)

	// Baseline Requirements v1.8.1 section 4.2.1: "any reused data, document,
	// or completed validation MUST be obtained no more than 398 days prior
	// to issuing the Certificate". If unconfigured or the configured value is
	// greater than 397 days, bail out.
	if c.RA.AuthorizationLifetimeDays <= 0 || c.RA.AuthorizationLifetimeDays > 397 {
		cmd.Fail("authorizationLifetimeDays value must be greater than 0 and less than 398")
	}
	authorizationLifetime := time.Duration(c.RA.AuthorizationLifetimeDays) * 24 * time.Hour

	// The Baseline Requirements v1.8.1 state that validation tokens "MUST
	// NOT be used for more than 30 days from its creation". If unconfigured
	// or the configured value pendingAuthorizationLifetimeDays is greater
	// than 29 days, bail out.
	if c.RA.PendingAuthorizationLifetimeDays <= 0 || c.RA.PendingAuthorizationLifetimeDays > 29 {
		cmd.Fail("pendingAuthorizationLifetimeDays value must be greater than 0 and less than 30")
	}
	pendingAuthorizationLifetime := time.Duration(c.RA.PendingAuthorizationLifetimeDays) * 24 * time.Hour

	if features.Get().AsyncFinalize && c.RA.FinalizeTimeout.Duration == 0 {
		cmd.Fail("finalizeTimeout must be supplied when AsyncFinalize feature is enabled")
	}

	kp, err := sagoodkey.NewPolicy(&c.RA.GoodKey, sac.KeyBlocked)
	cmd.FailOnError(err, "Unable to create key policy")

	if c.RA.MaxNames == 0 {
		cmd.Fail("Error in RA config: MaxNames must not be 0")
	}

	var limiter *ratelimits.Limiter
	var txnBuilder *ratelimits.TransactionBuilder
	var limiterRedis *bredis.Ring
	if c.RA.Limiter.Defaults != "" {
		// Setup rate limiting.
		limiterRedis, err = bredis.NewRingFromConfig(*c.RA.Limiter.Redis, scope, logger)
		cmd.FailOnError(err, "Failed to create Redis ring")

		source := ratelimits.NewRedisSource(limiterRedis.Ring, clk, scope)
		limiter, err = ratelimits.NewLimiter(clk, source, scope)
		cmd.FailOnError(err, "Failed to create rate limiter")
		txnBuilder, err = ratelimits.NewTransactionBuilder(c.RA.Limiter.Defaults, c.RA.Limiter.Overrides)
		cmd.FailOnError(err, "Failed to create rate limits transaction builder")
	}

	rai := ra.NewRegistrationAuthorityImpl(
		clk,
		logger,
		scope,
		c.RA.MaxContactsPerRegistration,
		kp,
		limiter,
		txnBuilder,
		c.RA.MaxNames,
		authorizationLifetime,
		pendingAuthorizationLifetime,
		pubc,
		caaClient,
		c.RA.OrderLifetime.Duration,
		c.RA.FinalizeTimeout.Duration,
		ctp,
		apc,
		issuerCerts,
	)
	defer rai.DrainFinalize()

	policyErr := rai.LoadRateLimitPoliciesFile(c.RA.RateLimitPoliciesFilename)
	cmd.FailOnError(policyErr, "Couldn't load rate limit policies file")
	rai.PA = pa

	rai.VA = vac
	rai.CA = cac
	rai.OCSP = ocspc
	rai.SA = sac

	start, err := bgrpc.NewServer(c.RA.GRPC, logger).Add(
		&rapb.RegistrationAuthority_ServiceDesc, rai).Build(tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to setup RA gRPC server")

	cmd.FailOnError(start(), "RA gRPC service failed")
}

func init() {
	cmd.RegisterCommand("boulder-ra", main, &cmd.ConfigValidator{Config: &Config{}})
}
