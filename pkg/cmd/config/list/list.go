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
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print a list of configuration keys and values",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Config = f.Config

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "host", "h", "", "Get per-host setting")

	return cmd
}

func listRun(opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	//cs := opts.IO.ColorScheme()
	isTTY := opts.IO.IsStdoutTTY()

	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	if isTTY {
		fmt.Printf("Settings configured for %s\n\n", host)
	}

	configOptions := config.ConfigOptions()

	for _, key := range configOptions {
		val, err := cfg.Get(opts.Hostname, key.Key)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", key.Key, val)
	}

	return nil
}
