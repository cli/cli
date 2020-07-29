package command

import (
	"fmt"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/spf13/cobra"
)

func prFromArgs(ctx context.Context, apiClient *api.Client, cmd *cobra.Command, args []string) (*api.PullRequest, ghrepo.Interface, error) {
	var arg string
	if len(args) > 0 {
		arg = args[0]
	}

	return shared.PRFromArgs(
		apiClient,
		func() (ghrepo.Interface, error) {
			repo, err := determineBaseRepo(apiClient, cmd, ctx)
			if err != nil {
				return nil, fmt.Errorf("could not determine base repo: %w", err)
			}
			return repo, nil
		},
		func() (string, error) {
			return ctx.Branch()
		},
		func() (context.Remotes, error) {
			return ctx.Remotes()
		}, arg)
}
