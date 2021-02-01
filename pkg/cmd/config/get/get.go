package get

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/cmdutil/action"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

type GetOptions struct {
	IO     *iostreams.IOStreams
	Config config.Config

	Hostname string
	Key      string
}

func NewCmdConfigGet(f *cmdutil.Factory, runF func(*GetOptions) error) *cobra.Command {
	opts := &GetOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print the value of a given configuration key",
		Example: heredoc.Doc(`
			$ gh config get git_protocol
			https
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := f.Config()
			if err != nil {
				return err
			}
			opts.Config = config
			opts.Key = args[0]

			if runF != nil {
				return runF(opts)
			}

			return getRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "host", "h", "", "Get per-host setting")

	carapace.Gen(cmd).FlagCompletion(carapace.ActionMap{
		"host": action.ActionConfigHosts(),
	})

	carapace.Gen(cmd).PositionalCompletion(
		carapace.ActionValuesDescribed(
			"git_protocol", "What protocol to use when performing git operations.",
			"editor", "What editor gh should run when creating issues, pull requests, etc.",
			"prompt", "toggle interactive prompting in the terminal",
			"pager", "the terminal pager program to send standard output to",
		),
	)

	return cmd
}

func getRun(opts *GetOptions) error {
	val, err := opts.Config.Get(opts.Hostname, opts.Key)
	if err != nil {
		return err
	}

	if val != "" {
		fmt.Fprintf(opts.IO.Out, "%s\n", val)
	}
	return nil
}
