package list

import (
	"fmt"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config.Config, error)

	Hostname string
}

func NewCmdConfigList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print a list of configuration keys and values",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "host", "h", "", "Get per-host configuration")

	return cmd
}

func listRun(opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	isTTY := opts.IO.IsStdoutTTY()

	var host string
	if opts.Hostname != "" {
		host = opts.Hostname
	} else {
		host, err = cfg.DefaultHost()
		if err != nil {
			return err
		}
	}

	if isTTY {
		fmt.Fprint(opts.IO.Out, cs.Grayf("Settings configured for %s\n\n", host))
	}

	configOptions := config.ConfigOptions()

	for _, key := range configOptions {
		val, err := cfg.Get(opts.Hostname, key.Key)
		if err != nil {
			return err
		}
		configLine := fmt.Sprintf("%s: %s", key.Key, val)
		if isTTY && key.DefaultValue != "" && val != key.DefaultValue {
			configLine = cs.Bold(configLine)
			configLine += fmt.Sprintf(" (default: %s)", key.DefaultValue)
		}
		fmt.Fprint(opts.IO.Out, configLine, "\n")
	}

	return nil
}
