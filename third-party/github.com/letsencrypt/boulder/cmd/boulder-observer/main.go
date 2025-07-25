package notmain

import (
	"flag"
	"os"

	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/observer"
	"github.com/letsencrypt/boulder/strictyaml"
)

func main() {
	debugAddr := flag.String("debug-addr", "", "Debug server address override")
	configPath := flag.String(
		"config", "config.yml", "Path to boulder-observer configuration file")
	flag.Parse()

	configYAML, err := os.ReadFile(*configPath)
	cmd.FailOnError(err, "failed to read config file")

	// Parse the YAML config file.
	var config observer.ObsConf
	err = strictyaml.Unmarshal(configYAML, &config)

	if *debugAddr != "" {
		config.DebugAddr = *debugAddr
	}

	if err != nil {
		cmd.FailOnError(err, "failed to parse YAML config")
	}

	// Make an `Observer` object.
	observer, err := config.MakeObserver()
	if err != nil {
		cmd.FailOnError(err, "config failed validation")
	}

	// Start the `Observer` daemon.
	observer.Start()
}

func init() {
	cmd.RegisterCommand("boulder-observer", main, &cmd.ConfigValidator{Config: &observer.ObsConf{}})
}
