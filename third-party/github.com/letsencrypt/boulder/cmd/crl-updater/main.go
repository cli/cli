package notmain

import (
	"context"
	"errors"
	"flag"
	"os"
	"time"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	cspb "github.com/letsencrypt/boulder/crl/storer/proto"
	"github.com/letsencrypt/boulder/crl/updater"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/issuance"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

type Config struct {
	CRLUpdater struct {
		DebugAddr string `validate:"omitempty,hostname_port"`

		// TLS client certificate, private key, and trusted root bundle.
		TLS cmd.TLSConfig

		SAService           *cmd.GRPCClientConfig
		CRLGeneratorService *cmd.GRPCClientConfig
		CRLStorerService    *cmd.GRPCClientConfig

		// IssuerCerts is a list of paths to issuer certificates on disk. This
		// controls the set of CRLs which will be published by this updater: it will
		// publish one set of NumShards CRL shards for each issuer in this list.
		IssuerCerts []string `validate:"min=1,dive,required"`

		// NumShards is the number of shards into which each issuer's "full and
		// complete" CRL will be split.
		// WARNING: When this number is changed, the "JSON Array of CRL URLs" field
		// in CCADB MUST be updated.
		NumShards int `validate:"min=1"`

		// ShardWidth is the amount of time (width on a timeline) that a single
		// shard should cover. Ideally, NumShards*ShardWidth should be an amount of
		// time noticeably larger than the current longest certificate lifetime,
		// but the updater will continue to work if this is not the case (albeit
		// with more confusing mappings of serials to shards).
		// WARNING: When this number is changed, revocation entries will move
		// between shards.
		ShardWidth config.Duration `validate:"-"`

		// LookbackPeriod is how far back the updater should look for revoked expired
		// certificates. We are required to include every revoked cert in at least
		// one CRL, even if it is revoked seconds before it expires, so this must
		// always be greater than the UpdatePeriod, and should be increased when
		// recovering from an outage to ensure continuity of coverage.
		LookbackPeriod config.Duration `validate:"-"`

		// UpdatePeriod controls how frequently the crl-updater runs and publishes
		// new versions of every CRL shard. The Baseline Requirements, Section 4.9.7:
		// "MUST update and publish a new CRL within twenty‐four (24) hours after
		// recording a Certificate as revoked."
		UpdatePeriod config.Duration

		// UpdateTimeout controls how long a single CRL shard is allowed to attempt
		// to update before being timed out. The total CRL updating process may take
		// significantly longer, since a full update cycle may consist of updating
		// many shards with varying degrees of parallelism. This value must be
		// strictly less than the UpdatePeriod. Defaults to 10 minutes, one order
		// of magnitude greater than our p99 update latency.
		UpdateTimeout config.Duration `validate:"-"`

		// TemporallyShardedSerialPrefixes is a list of prefixes that were used to
		// issue certificates with no CRLDistributionPoints extension, and which are
		// therefore temporally sharded. If it's non-empty, the CRL Updater will
		// require matching serials when querying by temporal shard. When querying
		// by explicit shard, any prefix is allowed.
		//
		// This should be set to the current set of serial prefixes in production.
		// When deploying explicit sharding (i.e. the CRLDistributionPoints extension),
		// the CAs should be configured with a new set of serial prefixes that haven't
		// been used before (and the OCSP Responder config should be updated to
		// recognize the new prefixes as well as the old ones).
		TemporallyShardedSerialPrefixes []string

		// MaxParallelism controls how many workers may be running in parallel.
		// A higher value reduces the total time necessary to update all CRL shards
		// that this updater is responsible for, but also increases the memory used
		// by this updater. Only relevant in -runOnce mode.
		MaxParallelism int `validate:"min=0"`

		// MaxAttempts control how many times the updater will attempt to generate
		// a single CRL shard. A higher number increases the likelihood of a fully
		// successful run, but also increases the worst-case runtime and db/network
		// load of said run. The default is 1.
		MaxAttempts int `validate:"omitempty,min=1"`

		// ExpiresMargin adds a small increment to the CRL's HTTP Expires time.
		//
		// When uploading a CRL, its Expires field in S3 is set to the expected time
		// the next CRL will be uploaded (by this instance). That allows our CDN
		// instances to cache for that long. However, since the next update might be
		// slow or delayed, we add a margin of error.
		//
		// Tradeoffs: A large ExpiresMargin reduces the chance that a CRL becomes
		// uncacheable and floods S3 with traffic (which might result in 503s while
		// S3 scales out).
		//
		// A small ExpiresMargin means revocations become visible sooner, including
		// admin-invoked revocations that may have a time requirement.
		ExpiresMargin config.Duration

		// CacheControl is a string passed verbatim to the crl-storer to store on
		// the S3 object.
		//
		// Note: if this header contains max-age, it will override
		// Expires. https://www.rfc-editor.org/rfc/rfc9111.html#name-calculating-freshness-lifet
		// Cache-Control: max-age has the disadvantage that it caches for a fixed
		// amount of time, regardless of how close the CRL is to replacement. So
		// if max-age is used, the worst-case time for a revocation to become visible
		// is UpdatePeriod + the value of max age.
		//
		// The stale-if-error and stale-while-revalidate headers may be useful here:
		// https://aws.amazon.com/about-aws/whats-new/2023/05/amazon-cloudfront-stale-while-revalidate-stale-if-error-cache-control-directives/
		//
		// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
		CacheControl string

		Features features.Config
	}

	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig
}

func main() {
	configFile := flag.String("config", "", "File path to the configuration file for this service")
	debugAddr := flag.String("debug-addr", "", "Debug server address override")
	runOnce := flag.Bool("runOnce", false, "If true, run once immediately and then exit")
	flag.Parse()
	if *configFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	var c Config
	err := cmd.ReadConfigFile(*configFile, &c)
	cmd.FailOnError(err, "Reading JSON config file into config structure")

	if *debugAddr != "" {
		c.CRLUpdater.DebugAddr = *debugAddr
	}

	features.Set(c.CRLUpdater.Features)

	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.CRLUpdater.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())
	clk := cmd.Clock()

	tlsConfig, err := c.CRLUpdater.TLS.Load(scope)
	cmd.FailOnError(err, "TLS config")

	issuers := make([]*issuance.Certificate, 0, len(c.CRLUpdater.IssuerCerts))
	for _, filepath := range c.CRLUpdater.IssuerCerts {
		cert, err := issuance.LoadCertificate(filepath)
		cmd.FailOnError(err, "Failed to load issuer cert")
		issuers = append(issuers, cert)
	}

	if c.CRLUpdater.ShardWidth.Duration == 0 {
		c.CRLUpdater.ShardWidth.Duration = 16 * time.Hour
	}
	if c.CRLUpdater.LookbackPeriod.Duration == 0 {
		c.CRLUpdater.LookbackPeriod.Duration = 24 * time.Hour
	}
	if c.CRLUpdater.UpdateTimeout.Duration == 0 {
		c.CRLUpdater.UpdateTimeout.Duration = 10 * time.Minute
	}

	saConn, err := bgrpc.ClientSetup(c.CRLUpdater.SAService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to SA")
	sac := sapb.NewStorageAuthorityClient(saConn)

	caConn, err := bgrpc.ClientSetup(c.CRLUpdater.CRLGeneratorService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to CRLGenerator")
	cac := capb.NewCRLGeneratorClient(caConn)

	csConn, err := bgrpc.ClientSetup(c.CRLUpdater.CRLStorerService, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to CRLStorer")
	csc := cspb.NewCRLStorerClient(csConn)

	u, err := updater.NewUpdater(
		issuers,
		c.CRLUpdater.NumShards,
		c.CRLUpdater.ShardWidth.Duration,
		c.CRLUpdater.LookbackPeriod.Duration,
		c.CRLUpdater.UpdatePeriod.Duration,
		c.CRLUpdater.UpdateTimeout.Duration,
		c.CRLUpdater.MaxParallelism,
		c.CRLUpdater.MaxAttempts,
		c.CRLUpdater.CacheControl,
		c.CRLUpdater.ExpiresMargin.Duration,
		c.CRLUpdater.TemporallyShardedSerialPrefixes,
		sac,
		cac,
		csc,
		scope,
		logger,
		clk,
	)
	cmd.FailOnError(err, "Failed to create crl-updater")

	ctx, cancel := context.WithCancel(context.Background())
	go cmd.CatchSignals(cancel)

	if *runOnce {
		err = u.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			cmd.FailOnError(err, "")
		}
	} else {
		err = u.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			cmd.FailOnError(err, "")
		}
	}
}

func init() {
	cmd.RegisterCommand("crl-updater", main, &cmd.ConfigValidator{Config: &Config{}})
}
