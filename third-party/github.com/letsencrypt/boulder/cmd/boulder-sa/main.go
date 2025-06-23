package notmain

import (
	"context"
	"flag"
	"os"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/sa"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

type Config struct {
	SA struct {
		cmd.ServiceConfig
		DB          cmd.DBConfig
		ReadOnlyDB  cmd.DBConfig `validate:"-"`
		IncidentsDB cmd.DBConfig `validate:"-"`

		Features features.Config

		// Max simultaneous SQL queries caused by a single RPC.
		ParallelismPerRPC int `validate:"omitempty,min=1"`
		// LagFactor is how long to sleep before retrying a read request that may
		// have failed solely due to replication lag.
		LagFactor config.Duration `validate:"-"`
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

	features.Set(c.SA.Features)

	if *grpcAddr != "" {
		c.SA.GRPC.Address = *grpcAddr
	}
	if *debugAddr != "" {
		c.SA.DebugAddr = *debugAddr
	}

	scope, logger, oTelShutdown := cmd.StatsAndLogging(c.Syslog, c.OpenTelemetry, c.SA.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())

	dbMap, err := sa.InitWrappedDb(c.SA.DB, scope, logger)
	cmd.FailOnError(err, "While initializing dbMap")

	dbReadOnlyMap := dbMap
	if c.SA.ReadOnlyDB != (cmd.DBConfig{}) {
		dbReadOnlyMap, err = sa.InitWrappedDb(c.SA.ReadOnlyDB, scope, logger)
		cmd.FailOnError(err, "While initializing dbReadOnlyMap")
	}

	dbIncidentsMap := dbMap
	if c.SA.IncidentsDB != (cmd.DBConfig{}) {
		dbIncidentsMap, err = sa.InitWrappedDb(c.SA.IncidentsDB, scope, logger)
		cmd.FailOnError(err, "While initializing dbIncidentsMap")
	}

	clk := cmd.Clock()

	parallel := c.SA.ParallelismPerRPC
	if parallel < 1 {
		parallel = 1
	}

	tls, err := c.SA.TLS.Load(scope)
	cmd.FailOnError(err, "TLS config")

	saroi, err := sa.NewSQLStorageAuthorityRO(
		dbReadOnlyMap, dbIncidentsMap, scope, parallel, c.SA.LagFactor.Duration, clk, logger)
	cmd.FailOnError(err, "Failed to create read-only SA impl")

	sai, err := sa.NewSQLStorageAuthorityWrapping(saroi, dbMap, scope)
	cmd.FailOnError(err, "Failed to create SA impl")

	start, err := bgrpc.NewServer(c.SA.GRPC, logger).WithCheckInterval(c.SA.HealthCheckInterval.Duration).Add(
		&sapb.StorageAuthorityReadOnly_ServiceDesc, saroi).Add(
		&sapb.StorageAuthority_ServiceDesc, sai).Build(
		tls, scope, clk)
	cmd.FailOnError(err, "Unable to setup SA gRPC server")

	cmd.FailOnError(start(), "SA gRPC service failed")
}

func init() {
	cmd.RegisterCommand("boulder-sa", main, &cmd.ConfigValidator{Config: &Config{}})
}
