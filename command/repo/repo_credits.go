package repo

import (
	"github.com/cli/cli/command"
	"github.com/spf13/cobra"
)

func RepoCreditsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "credits [<repository>]",
		Short:  "View credits for a repository",
		Long:   `View credits for a repository`,
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE:   repoCredits,
	}

	cmd.Flags().BoolP("static", "s", false, "Print a static version of the credits")

	return cmd
}

func repoCredits(cmd *cobra.Command, args []string) error {
	return command.Credits(cmd, args)	
}
