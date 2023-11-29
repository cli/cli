package token

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type TokenOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config.Config, error)

	Hostname      string
	SecureStorage bool
}

// TODO: Detmerine if this is wonky in multi-account world. Do we need a --user flag?
func NewCmdToken(f *cmdutil.Factory, runF func(*TokenOptions) error) *cobra.Command {
	opts := &TokenOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authentication token gh is configured to use",
		Long: heredoc.Doc(`
			This command outputs the authentication token for the active
			account on a given GitHub host.
		`),
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return tokenRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance authenticated with")
	cmd.Flags().BoolVarP(&opts.SecureStorage, "secure-storage", "", false, "Search only secure credential store for authentication token")
	_ = cmd.Flags().MarkHidden("secure-storage")

	return cmd
}

func tokenRun(opts *TokenOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	hostname := opts.Hostname
	if hostname == "" {
		hostname, _ = authCfg.DefaultHost()
	}

	var val string
	if opts.SecureStorage {
		val, _ = authCfg.TokenFromKeyring(hostname)
	} else {
		val, _ = authCfg.Token(hostname)
	}
	if val == "" {
		return fmt.Errorf("no oauth token")
	}

	if val != "" {
		fmt.Fprintf(opts.IO.Out, "%s\n", val)
	}
	return nil
}
