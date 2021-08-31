package setupgit

import (
	"fmt"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmd/auth/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type gitConfigurator interface {
	Setup(hostname, username, authToken string) error
}

type SetupGitOptions struct {
	IO *iostreams.IOStreams
	// Hostname is the host to setup as git credential helper for. This is set via command flag.
	Hostname     string
	Config       func() (config.Config, error)
	gitConfigure gitConfigurator
}

func NewCmdSetupGit(f *cmdutil.Factory, runF func(*SetupGitOptions) error) *cobra.Command {
	opts := &SetupGitOptions{
		IO:           f.IOStreams,
		Config:       f.Config,
		gitConfigure: &shared.GitCredentialFlow{},
	}

	cmd := &cobra.Command{
		Short: "Setup GH CLI as a git credential helper",
		Long:  "Setup GH CLI as a git credential helper for all authenticated hostnames.",
		Use:   "setup-git [<hostname>]",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return setupGitRun(opts)
		},
	}

	cmd.Flags().StringVarP(
		&opts.Hostname,
		"hostname",
		"h",
		"",
		"Setup git credential helper for a specific hostname",
	)

	return cmd
}

func setupGitRun(opts *SetupGitOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hostnames, err := cfg.Hosts()
	if err != nil {
		return err
	}

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
		if !utils.Has(opts.Hostname, hostnames) {
			fmt.Fprintf(
				stderr,
				"You are not logged into any Github host with the hostname %s\n",
				opts.Hostname,
			)
			return cmdutil.SilentError
		}
		hostnamesToSetup = []string{opts.Hostname}
	}

	if err := setupGitForHostnames(hostnamesToSetup, opts.gitConfigure); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return cmdutil.SilentError
	}

	return nil
}

func setupGitForHostnames(hostnames []string, gitCfg gitConfigurator) error {
	for _, hostname := range hostnames {
		if err := gitCfg.Setup(hostname, "", ""); err != nil {
			return fmt.Errorf("failed to setup git credential helper: %w", err)
		}
	}
	return nil
}
