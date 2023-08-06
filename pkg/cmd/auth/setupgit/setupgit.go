package setupgit

import (
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type gitConfigurator interface {
	Setup(hostname, username, authToken string) error
}

type SetupGitOptions struct {
	IO           *iostreams.IOStreams
	Config       func() (config.Config, error)
	Hostname     string
	gitConfigure gitConfigurator
}

func NewCmdSetupGit(f *cmdutil.Factory, runF func(*SetupGitOptions) error) *cobra.Command {
	opts := &SetupGitOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "setup-git",
		Short: "Setup git with GitHub CLI",
		Long: heredoc.Docf(`
			This command configures git to use GitHub CLI as a credential helper.
			For more information on git credential helpers please reference:
			https://git-scm.com/docs/gitcredentials.

			By default, GitHub CLI will be set as the credential helper for all authenticated hosts.
			If there is no authenticated hosts the command fails with an error.

			Alternatively, use the %[1]s--hostname%[1]s flag to specify a single host to be configured.
			If the host is not authenticated with, the command fails with an error.
		`, "`"),
		Example: heredoc.Doc(`
			# Configure git to use GitHub CLI as the credential helper for all authenticated hosts
			$ gh auth setup-git

			# Configure git to use GitHub CLI as the credential helper for enterprise.internal host
			$ gh auth setup-git --hostname enterprise.internal
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.gitConfigure = &shared.GitCredentialFlow{
				Executable: f.Executable(),
				GitClient:  f.GitClient,
			}

			if runF != nil {
				return runF(opts)
			}
			return setupGitRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "The hostname to configure git for")

	return cmd
}

func setupGitRun(opts *SetupGitOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	hostnames := authCfg.Hosts()

	stderr := opts.IO.ErrOut
	cs := opts.IO.ColorScheme()

	if len(hostnames) == 0 {
		fmt.Fprintf(
			stderr,
			"You are not logged into any GitHub hosts. Run %s to authenticate.\n",
			cs.Bold("gh auth login"),
		)

		return cmdutil.SilentError
	}

	hostnamesToSetup := hostnames

	if opts.Hostname != "" {
		if !has(opts.Hostname, hostnames) {
			return fmt.Errorf("You are not logged into the GitHub host %q\n", opts.Hostname)
		}
		hostnamesToSetup = []string{opts.Hostname}
	}

	for _, hostname := range hostnamesToSetup {
		if err := opts.gitConfigure.Setup(hostname, "", ""); err != nil {
			return fmt.Errorf("failed to set up git credential helper: %w", err)
		}
	}

	return nil
}

func has(needle string, haystack []string) bool {
	for _, s := range haystack {
		if strings.EqualFold(s, needle) {
			return true
		}
	}
	return false
}
