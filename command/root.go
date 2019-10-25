package command

import (
	"fmt"
	"os"

	"github.com/github/gh-cli/context"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.PersistentFlags().StringP("repo", "R", "", "current GitHub repository")
	RootCmd.PersistentFlags().StringP("current-branch", "B", "", "current git branch")
}

// RootCmd is the entry point of command-line execution
var RootCmd = &cobra.Command{
	Use:   "gh",
	Short: "GitHub CLI",
	Long:  `Do things with GitHub from your terminal`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("root")
	},
}

func contextForCommand(cmd *cobra.Command) context.Context {
	ctx := context.New()
	if repo := os.Getenv("GH_REPO"); repo != "" {
		ctx.SetBaseRepo(repo)
	}
	if repo, err := cmd.Flags().GetString("repo"); err == nil {
		ctx.SetBaseRepo(repo)
	}
	if branch, err := cmd.Flags().GetString("current-branch"); err == nil {
		ctx.SetBranch(branch)
	}
	return ctx
}
