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

	var host string
	if opts.Hostname != "" {
		host = opts.Hostname
	} else {
		host, err = cfg.DefaultHost()
		if err != nil {
			return err
		}
	}

	configOptions := config.ConfigOptions()

	for _, key := range configOptions {
		val, err := cfg.Get(host, key.Key)
		if err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.Out, "%s=%s\n", key.Key, val)
	}

	return nil
}
