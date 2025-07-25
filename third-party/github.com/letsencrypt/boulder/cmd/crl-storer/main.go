package notmain

import (
	"context"
	"flag"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	awsl "github.com/aws/smithy-go/logging"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/crl/storer"
	cspb "github.com/letsencrypt/boulder/crl/storer/proto"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
)

type Config struct {
	CRLStorer struct {
		cmd.ServiceConfig

		// IssuerCerts is a list of paths to issuer certificates on disk. These will
		// be used to validate the CRLs received by this service before uploading
		// them.
		IssuerCerts []string `validate:"min=1,dive,required"`

		// S3Endpoint is the URL at which the S3-API-compatible object storage
		// service can be reached. This can be used to point to a non-Amazon storage
		// service, or to point to a fake service for testing. It should be left
		// blank by default.
		S3Endpoint string
		// S3Bucket is the AWS Bucket that uploads should go to. Must be created
		// (and have appropriate permissions set) beforehand.
		S3Bucket string
		// AWSConfigFile is the path to a file on disk containing an AWS config.
		// The format of the configuration file is specified at
		// https://docs.aws.amazon.com/sdkref/latest/guide/file-format.html.
		AWSConfigFile string
		// AWSCredsFile is the path to a file on disk containing AWS credentials.
		// The format of the credentials file is specified at
		// https://docs.aws.amazon.com/sdkref/latest/guide/file-format.html.
		AWSCredsFile string

		Features features.Config
	}

	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig
}

// awsLogger implements the github.com/aws/smithy-go/logging.Logger interface.
type awsLogger struct {
	blog.Logger
}

func (log awsLogger) Logf(c awsl.Classification, format string, v ...interface{}) {
	switch c {
	case awsl.Debug:
		log.Debugf(format, v...)
	case awsl.Warn:
		log.Warningf(format, v...)
	}
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

	features.Set(c.CRLStorer.Features)

	if *grpcAddr != "" {
		c.CRLStorer.GRPC.Address = *grpcAddr
	}
	if *debugAddr != "" {
		c.CRLStorer.DebugAddr = *debugAddr
	}

	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.CRLStorer.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())
	clk := cmd.Clock()

	tlsConfig, err := c.CRLStorer.TLS.Load(scope)
	cmd.FailOnError(err, "TLS config")

	issuers := make([]*issuance.Certificate, 0, len(c.CRLStorer.IssuerCerts))
	for _, filepath := range c.CRLStorer.IssuerCerts {
		cert, err := issuance.LoadCertificate(filepath)
		cmd.FailOnError(err, "Failed to load issuer cert")
		issuers = append(issuers, cert)
	}

	// Load the "default" AWS configuration, but override the set of config and
	// credential files it reads from to just those specified in our JSON config,
	// to ensure that it's not accidentally reading anything from the homedir or
	// its other default config locations.
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithSharedConfigFiles([]string{c.CRLStorer.AWSConfigFile}),
		config.WithSharedCredentialsFiles([]string{c.CRLStorer.AWSCredsFile}),
		config.WithHTTPClient(new(http.Client)),
		config.WithLogger(awsLogger{logger}),
		config.WithClientLogMode(aws.LogRequestEventMessage|aws.LogResponseEventMessage),
	)
	cmd.FailOnError(err, "Failed to load AWS config")

	s3opts := make([]func(*s3.Options), 0)
	if c.CRLStorer.S3Endpoint != "" {
		s3opts = append(
			s3opts,
			s3.WithEndpointResolver(s3.EndpointResolverFromURL(c.CRLStorer.S3Endpoint)),
			func(o *s3.Options) { o.UsePathStyle = true },
		)
	}
	s3client := s3.NewFromConfig(awsConfig, s3opts...)

	csi, err := storer.New(issuers, s3client, c.CRLStorer.S3Bucket, scope, logger, clk)
	cmd.FailOnError(err, "Failed to create CRLStorer impl")

	start, err := bgrpc.NewServer(c.CRLStorer.GRPC, logger).Add(
		&cspb.CRLStorer_ServiceDesc, csi).Build(tlsConfig, scope, clk)
	cmd.FailOnError(err, "Unable to setup CRLStorer gRPC server")

	cmd.FailOnError(start(), "CRLStorer gRPC service failed")
}

func init() {
	cmd.RegisterCommand("crl-storer", main, &cmd.ConfigValidator{Config: &Config{}})
}
