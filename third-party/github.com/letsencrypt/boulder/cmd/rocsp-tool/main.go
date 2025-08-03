package notmain

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/db"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/metrics"
	rocsp_config "github.com/letsencrypt/boulder/rocsp/config"
	"github.com/letsencrypt/boulder/sa"
	"github.com/letsencrypt/boulder/test/ocsp/helper"
)

type Config struct {
	ROCSPTool struct {
		DebugAddr string `validate:"omitempty,hostname_port"`
		Redis     rocsp_config.RedisConfig

		// If using load-from-db, this provides credentials to connect to the DB
		// and the CA. Otherwise, it's optional.
		LoadFromDB *LoadFromDBConfig
	}
	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig
}

// LoadFromDBConfig provides the credentials and configuration needed to load
// data from the certificateStatuses table in the DB and get it signed.
type LoadFromDBConfig struct {
	// Credentials to connect to the DB.
	DB cmd.DBConfig
	// Credentials to request OCSP signatures from the CA.
	GRPCTLS cmd.TLSConfig
	// Timeouts and hostnames for the CA.
	OCSPGeneratorService cmd.GRPCClientConfig
	// How fast to process rows.
	Speed ProcessingSpeed
}

type ProcessingSpeed struct {
	// If using load-from-db, this limits how many items per second we
	// scan from the DB. We might go slower than this depending on how fast
	// we read rows from the DB, but we won't go faster. Defaults to 2000.
	RowsPerSecond int `validate:"min=0"`
	// If using load-from-db, this controls how many parallel requests to
	// boulder-ca for OCSP signing we can make. Defaults to 100.
	ParallelSigns int `validate:"min=0"`
	// If using load-from-db, the LIMIT on our scanning queries. We have to
	// apply a limit because MariaDB will cut off our response at some
	// threshold of total bytes transferred (1 GB by default). Defaults to 10000.
	ScanBatchSize int `validate:"min=0"`
}

func init() {
	cmd.RegisterCommand("rocsp-tool", main, &cmd.ConfigValidator{Config: &Config{}})
}

func main() {
	err := main2()
	if err != nil {
		cmd.FailOnError(err, "")
	}
}

var startFromID *int64

func main2() error {
	debugAddr := flag.String("debug-addr", "", "Debug server address override")
	configFile := flag.String("config", "", "File path to the configuration file for this service")
	startFromID = flag.Int64("start-from-id", 0, "For load-from-db, the first ID in the certificateStatus table to scan")
	flag.Usage = helpExit
	flag.Parse()
	if *configFile == "" || len(flag.Args()) < 1 {
		helpExit()
	}

	var conf Config
	err := cmd.ReadConfigFile(*configFile, &conf)
	if err != nil {
		return fmt.Errorf("reading JSON config file: %w", err)
	}

	if *debugAddr != "" {
		conf.ROCSPTool.DebugAddr = *debugAddr
	}

	_, logger, oTelShutdown := cmd.StatsAndLogging(conf.Syslog, conf.OpenTelemetry, conf.ROCSPTool.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())

	clk := cmd.Clock()
	redisClient, err := rocsp_config.MakeClient(&conf.ROCSPTool.Redis, clk, metrics.NoopRegisterer)
	if err != nil {
		return fmt.Errorf("making client: %w", err)
	}

	var db *db.WrappedMap
	var ocspGenerator capb.OCSPGeneratorClient
	var scanBatchSize int
	if conf.ROCSPTool.LoadFromDB != nil {
		lfd := conf.ROCSPTool.LoadFromDB
		db, err = sa.InitWrappedDb(lfd.DB, nil, logger)
		if err != nil {
			return fmt.Errorf("connecting to DB: %w", err)
		}

		ocspGenerator, err = configureOCSPGenerator(lfd.GRPCTLS,
			lfd.OCSPGeneratorService, clk, metrics.NoopRegisterer)
		if err != nil {
			return fmt.Errorf("configuring gRPC to CA: %w", err)
		}
		setDefault(&lfd.Speed.RowsPerSecond, 2000)
		setDefault(&lfd.Speed.ParallelSigns, 100)
		setDefault(&lfd.Speed.ScanBatchSize, 10000)
		scanBatchSize = lfd.Speed.ScanBatchSize
	}

	ctx := context.Background()
	cl := client{
		redis:         redisClient,
		db:            db,
		ocspGenerator: ocspGenerator,
		clk:           clk,
		scanBatchSize: scanBatchSize,
		logger:        logger,
	}

	for _, sc := range subCommands {
		if flag.Arg(0) == sc.name {
			return sc.cmd(ctx, cl, conf, flag.Args()[1:])
		}
	}
	fmt.Fprintf(os.Stderr, "unrecognized subcommand %q\n", flag.Arg(0))
	helpExit()
	return nil
}

// subCommand represents a single subcommand. `name` is the name used to invoke it, and `help` is
// its help text.
type subCommand struct {
	name string
	help string
	cmd  func(context.Context, client, Config, []string) error
}

var (
	Store = subCommand{"store", "for each filename on command line, read the file as an OCSP response and store it in Redis",
		func(ctx context.Context, cl client, _ Config, args []string) error {
			err := cl.storeResponsesFromFiles(ctx, flag.Args()[1:])
			if err != nil {
				return err
			}
			return nil
		},
	}
	Get = subCommand{
		"get",
		"for each serial on command line, fetch that serial's response and pretty-print it",
		func(ctx context.Context, cl client, _ Config, args []string) error {
			for _, serial := range flag.Args()[1:] {
				resp, err := cl.redis.GetResponse(ctx, serial)
				if err != nil {
					return err
				}
				parsed, err := ocsp.ParseResponse(resp, nil)
				if err != nil {
					fmt.Fprintf(os.Stderr, "parsing error on %x: %s", resp, err)
					continue
				} else {
					fmt.Printf("%s\n", helper.PrettyResponse(parsed))
				}
			}
			return nil
		},
	}
	GetPEM = subCommand{"get-pem", "for each serial on command line, fetch that serial's response and print it PEM-encoded",
		func(ctx context.Context, cl client, _ Config, args []string) error {
			for _, serial := range flag.Args()[1:] {
				resp, err := cl.redis.GetResponse(ctx, serial)
				if err != nil {
					return err
				}
				block := pem.Block{
					Bytes: resp,
					Type:  "OCSP RESPONSE",
				}
				err = pem.Encode(os.Stdout, &block)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	LoadFromDB = subCommand{"load-from-db", "scan the database for all OCSP entries for unexpired certificates, and store in Redis",
		func(ctx context.Context, cl client, c Config, args []string) error {
			if c.ROCSPTool.LoadFromDB == nil {
				return fmt.Errorf("config field LoadFromDB was missing")
			}
			err := cl.loadFromDB(ctx, c.ROCSPTool.LoadFromDB.Speed, *startFromID)
			if err != nil {
				return fmt.Errorf("loading OCSP responses from DB: %w", err)
			}
			return nil
		},
	}
	ScanResponses = subCommand{"scan-responses", "scan Redis for OCSP response entries. For each entry, print the serial and base64-encoded response",
		func(ctx context.Context, cl client, _ Config, args []string) error {
			results := cl.redis.ScanResponses(ctx, "*")
			for r := range results {
				if r.Err != nil {
					return r.Err
				}
				fmt.Printf("%s: %s\n", r.Serial, base64.StdEncoding.EncodeToString(r.Body))
			}
			return nil
		},
	}
)

var subCommands = []subCommand{
	Store, Get, GetPEM, LoadFromDB, ScanResponses,
}

func helpExit() {
	var names []string
	var helpStrings []string
	for _, s := range subCommands {
		names = append(names, s.name)
		helpStrings = append(helpStrings, fmt.Sprintf("  %s -- %s", s.name, s.help))
	}
	fmt.Fprintf(os.Stderr, "Usage: %s [%s] --config path/to/config.json\n", os.Args[0], strings.Join(names, "|"))
	os.Stderr.Write([]byte(strings.Join(helpStrings, "\n")))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr)
	flag.PrintDefaults()
	os.Exit(1)
}

func configureOCSPGenerator(tlsConf cmd.TLSConfig, grpcConf cmd.GRPCClientConfig, clk clock.Clock, scope prometheus.Registerer) (capb.OCSPGeneratorClient, error) {
	tlsConfig, err := tlsConf.Load(scope)
	if err != nil {
		return nil, fmt.Errorf("loading TLS config: %w", err)
	}

	caConn, err := bgrpc.ClientSetup(&grpcConf, tlsConfig, scope, clk)
	cmd.FailOnError(err, "Failed to load credentials and create gRPC connection to CA")
	return capb.NewOCSPGeneratorClient(caConn), nil
}

// setDefault sets the target to a default value, if it is zero.
func setDefault(target *int, def int) {
	if *target == 0 {
		*target = def
	}
}
