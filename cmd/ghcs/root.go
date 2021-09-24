package ghcs

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/lightstep/lightstep-tracer-go"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
)

var version = "DEV" // Replaced in the release build process (by GoReleaser or Homebrew) by the git tag version number.

func NewRootCmd(app *App) *cobra.Command {
	var lightstep string

	root := &cobra.Command{
		Use:           "ghcs",
		SilenceUsage:  true,  // don't print usage message after each error (see #80)
		SilenceErrors: false, // print errors automatically so that main need not
		Long: `Unofficial CLI tool to manage GitHub Codespaces.

Running commands requires the GITHUB_TOKEN environment variable to be set to a
token to access the GitHub API with.`,
		Version: version,

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initLightstep(lightstep)
		},
	}

	root.PersistentFlags().StringVar(&lightstep, "lightstep", "", "Lightstep tracing endpoint (service:token@host:port)")

	root.AddCommand(newCodeCmd(app))
	root.AddCommand(newCreateCmd(app))
	root.AddCommand(newDeleteCmd(app))
	root.AddCommand(newListCmd(app))
	root.AddCommand(newLogsCmd(app))
	root.AddCommand(newPortsCmd(app))
	root.AddCommand(newSSHCmd(app))

	return root
}

// initLightstep parses the --lightstep=service:token@host:port flag and
// enables tracing if non-empty.
func initLightstep(config string) error {
	if config == "" {
		return nil
	}

	cut := func(s, sep string) (pre, post string) {
		if i := strings.Index(s, sep); i >= 0 {
			return s[:i], s[i+len(sep):]
		}
		return s, ""
	}

	// Parse service:token@host:port.
	serviceToken, hostPort := cut(config, "@")
	service, token := cut(serviceToken, ":")
	host, port := cut(hostPort, ":")
	portI, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid Lightstep configuration: %s", config)
	}

	opentracing.SetGlobalTracer(lightstep.NewTracer(lightstep.Options{
		AccessToken: token,
		Collector: lightstep.Endpoint{
			Host:      host,
			Port:      portI,
			Plaintext: false,
		},
		Tags: opentracing.Tags{
			lightstep.ComponentNameKey: service,
		},
	}))

	// Report failure to record traces.
	lightstep.SetGlobalEventHandler(func(ev lightstep.Event) {
		switch ev := ev.(type) {
		case lightstep.EventStatusReport, lightstep.MetricEventStatusReport:
			// ignore
		default:
			log.Printf("[trace] %s", ev)
		}
	})

	return nil
}
