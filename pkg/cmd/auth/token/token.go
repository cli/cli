package token

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type TokenOptions struct {
	IO     *iostreams.IOStreams
	Config func() (gh.Config, error)

	Hostname      string
	Username      string
	SecureStorage bool
}

func NewCmdToken(f *cmdutil.Factory, runF func(*TokenOptions) error) *cobra.Command {
	opts := &TokenOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authentication token gh uses for a hostname and account",
		Long: heredoc.Docf(`
			This command outputs the authentication token for an account on a given GitHub host.

			Without the %[1]s--hostname%[1]s flag, the default host is chosen.

			Without the %[1]s--user%[1]s flag, the active account for the host is chosen.
		`, "`"),
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return tokenRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname of the GitHub instance authenticated with")
	cmd.Flags().StringVarP(&opts.Username, "user", "u", "", "The account to output the token for")
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
	// If this conditional logic ends up being duplicated anywhere,
	// we should consider making a factory function that returns the correct
	// behavior. For now, keeping it all inline is simplest.
	if opts.SecureStorage {
		if opts.Username == "" {
			val, _ = authCfg.TokenFromKeyring(hostname)
		} else {
			val, _ = authCfg.TokenFromKeyringForUser(hostname, opts.Username)
		}
	} else {
		if opts.Username == "" {
			val, _ = authCfg.ActiveToken(hostname)
		} else {
			val, _, _ = authCfg.TokenForUser(hostname, opts.Username)
		}
	}

	if val == "" {
		errMsg := fmt.Sprintf("no oauth token found for %s", hostname)
		if opts.Username != "" {
			errMsg += fmt.Sprintf(" account %s", opts.Username)
		}
		return fmt.Errorf(errMsg)
	}

	if val != "" {
		fmt.Fprintf(opts.IO.Out, "%s\n", val)
	}

	return nil
}
