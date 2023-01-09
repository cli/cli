package setupgit

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAuth "github.com/cli/go-gh/pkg/auth"
	"github.com/spf13/cobra"
)

type gitConfigurator interface {
	Setup(hostname, username, authToken string) error
}

type SetupGitOptions struct {
	HttpClient     func() (*http.Client, error)
	IO             *iostreams.IOStreams
	Config         func() (config.Config, error)
	Hostname       string
	credentialFlow *shared.GitCredentialFlow
	gitConfigure   gitConfigurator
}

func NewCmdSetupGit(f *cmdutil.Factory, runF func(*SetupGitOptions) error) *cobra.Command {
	opts := &SetupGitOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Short: "Configure git to use GitHub CLI as a credential helper",
		Use:   "setup-git",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.credentialFlow = &shared.GitCredentialFlow{
				Executable: f.Executable(),
				GitClient:  f.GitClient,
			}
			opts.gitConfigure = opts.credentialFlow

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

	hostnames := cfg.Hosts()

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
	defaultHostname, _ := ghAuth.DefaultHost()

	if opts.Hostname != "" {
		if !has(opts.Hostname, hostnames) {
			return fmt.Errorf("You are not logged into the GitHub host %q\n", opts.Hostname)
		}
		hostnamesToSetup = []string{opts.Hostname}
		defaultHostname = opts.Hostname
	}

	for _, hostname := range hostnamesToSetup {
		if err := opts.gitConfigure.Setup(hostname, "", ""); err != nil {
			return fmt.Errorf("failed to set up git credential helper: %w", err)
		}

		if hostname == defaultHostname {
			ctx := context.Background()
			httpClient, err := opts.HttpClient()
			if err != nil {
				return err
			}
			apiClient := api.NewClientFromHTTP(httpClient)

			name, err := api.CurrentProfileName(apiClient, hostname)
			if err != nil {
				return fmt.Errorf("error using api: %w", err)
			}

			err = setGlobalGitConfig(ctx, opts.credentialFlow, "user.name", name)
			if err != nil {
				return err
			}
			fmt.Fprintf(opts.IO.ErrOut, "%s Set global git config user.name as %s\n", cs.SuccessIcon(), name)

			//TODO: repeat for email (token doesn't have required scope)
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

func setGlobalGitConfig(ctx context.Context, credentialFlow *shared.GitCredentialFlow, key, value string) error {
	cmd, err := credentialFlow.GitClient.Command(ctx, "config", "--global", key, value)
	if err != nil {
		return err
	}
	return cmd.Run()
}
