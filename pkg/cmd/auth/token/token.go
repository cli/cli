package token

import (
	"fmt"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type TokenOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config.Config, error)

	Hostname string
}

func NewCmdToken(f *cmdutil.Factory, runF func(*TokenOptions) error) *cobra.Command {
	opts := &TokenOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the auth token gh is configured to use",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return tokenRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance authenticated with")

	return cmd
}

func tokenRun(opts *TokenOptions) error {
	hostname := opts.Hostname
	if hostname == "" {
		hostname = ghinstance.Default()
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	key := "oauth_token"
	val, err := cfg.GetOrDefault(hostname, key)
	if err != nil {
		return fmt.Errorf("no oauth token")
	}

	if val != "" {
		fmt.Fprintf(opts.IO.Out, "%s\n", val)
	}
	return nil
}
