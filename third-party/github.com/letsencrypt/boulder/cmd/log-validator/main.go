package notmain

import (
	"context"
	"flag"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/log/validator"
)

type Config struct {
	Files         []string `validate:"min=1,dive,required"`
	DebugAddr     string   `validate:"omitempty,hostname_port"`
	Syslog        cmd.SyslogConfig
	OpenTelemetry cmd.OpenTelemetryConfig
}

func main() {
	debugAddr := flag.String("debug-addr", "", "Debug server address override")
	configFile := flag.String("config", "", "File path to the configuration file for this service")
	checkFile := flag.String("check-file", "", "File path to a file to directly validate, if this argument is provided the config will not be parsed and only this file will be inspected")
	flag.Parse()

	if *checkFile != "" {
		err := validator.ValidateFile(*checkFile)
		cmd.FailOnError(err, "validation failed")
		return
	}

	var config Config
	err := cmd.ReadConfigFile(*configFile, &config)
	cmd.FailOnError(err, "Reading JSON config file into config structure")

	if *debugAddr != "" {
		config.DebugAddr = *debugAddr
	}

	stats, logger, oTelShutdown := cmd.StatsAndLogging(config.Syslog, config.OpenTelemetry, config.DebugAddr)
	defer oTelShutdown(context.Background())
	logger.Info(cmd.VersionString())

	v := validator.New(config.Files, logger, stats)
	defer v.Shutdown()

	cmd.WaitForSignal()
}

func init() {
	cmd.RegisterCommand("log-validator", main, &cmd.ConfigValidator{Config: &Config{}})
}
