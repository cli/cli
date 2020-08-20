package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ChecksOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
	Remotes    func() (context.Remotes, error)

	HasRepoOverride bool

	// TODO cli options
}

func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	opts := &ChecksOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Branch:     f.Branch,
		Remotes:    f.Remotes,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "checks",
		Short: "Show CI status for a single pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			if runF != nil {
				return runF(opts)
			}

			return checksRun(opts)
		},
	}

	return cmd
}

func checksRun(opts *ChecksOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	var currentBranch string
	var currentPRNumber int
	var currentPRHeadRef string

	if !opts.HasRepoOverride {
		currentBranch, err = opts.Branch()
		if err != nil && !errors.Is(err, git.ErrNotOnAnyBranch) {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}

		remotes, _ := opts.Remotes()
		currentPRNumber, currentPRHeadRef, err = prSelectorForCurrentBranch(baseRepo, currentBranch, remotes)
		if err != nil {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}
	}

	// TODO get boiler plate finished, produce current or provided PR

	// TODO checks query

	// TODO checks formatting

	return nil
}
