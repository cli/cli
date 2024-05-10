package get

import (
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type GetOptions struct {
	IO     *iostreams.IOStreams
	Config gh.Config

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

	return cmd
}

func getRun(opts *GetOptions) error {
	// search keyring storage when fetching the `oauth_token` value
	if opts.Hostname != "" && opts.Key == "oauth_token" {
		token, _ := opts.Config.Authentication().ActiveToken(opts.Hostname)
		if token == "" {
			return errors.New(`could not find key "oauth_token"`)
		}
		fmt.Fprintf(opts.IO.Out, "%s\n", token)
		return nil
	}

	optionalEntry := opts.Config.GetOrDefault(opts.Hostname, opts.Key)
	if optionalEntry.IsNone() {
		return nonExistentKeyError{key: opts.Key}
	}

	val := optionalEntry.Unwrap().Value
	if val != "" {
		fmt.Fprintf(opts.IO.Out, "%s\n", val)
	}
	return nil
}

type nonExistentKeyError struct {
	key string
}

func (e nonExistentKeyError) Error() string {
	return fmt.Sprintf("could not find key \"%s\"", e.key)
}
