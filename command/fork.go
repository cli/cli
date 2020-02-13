package command

import (
	"fmt"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(forkCmd)
}

var forkCmd = &cobra.Command{
	Use:   "fork",
	Short: "fork repository in your account",
	Long:  `Fork with GitHub pull requests.`,
	RunE:  fork,
}

func fork(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	client, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return err
	}

	_, err = api.ForkRepo(client, *baseRepo)
	if err != nil {
		return fmt.Errorf("error fork remote %s : %w", ghrepo.FullName(*baseRepo), err)
	}
	return nil
}
