package command

import (
	"fmt"
	"os"

	"github.com/github/gh-cli/context"
	"github.com/spf13/cobra"
)

var (
	currentRepo   string
	currentBranch string
)

func init() {
	RootCmd.PersistentFlags().StringVarP(&currentRepo, "repo", "R", "", "current GitHub repository")
	RootCmd.PersistentFlags().StringVarP(&currentBranch, "current-branch", "B", "", "current git branch")
}

func initContext() {
	ctx := context.InitDefaultContext()
	ctx.SetBranch(currentBranch)
	repo := currentRepo
	if repo == "" {
		repo = os.Getenv("GH_REPO")
	}
	ctx.SetBaseRepo(repo)
}

// RootCmd is the entry point of command-line execution
var RootCmd = &cobra.Command{
	Use:   "gh",
	Short: "GitHub CLI",
	Long:  `Do things with GitHub from your terminal`,
	Args:  cobra.MinimumNArgs(1),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initContext()
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("root")
	},
}
