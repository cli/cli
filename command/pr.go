package command

import (
	"fmt"

	"github.com/github/gh-cli/graphql"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(prCmd)
	prCmd.AddCommand(prListCmd)
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Work with pull requests",
	Long: `This command allows you to
work with pull requests.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("pr")
	},
}

var prListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pull requests",
	Run: func(cmd *cobra.Command, args []string) {
		ExecutePr()
	},
}

func ExecutePr() {
	prPayload, err := graphql.PullRequests()
	if err != nil {
		panic(err)
	}

	if prPayload.CurrentPR != nil {
		fmt.Printf("Current Pr\n")
		printPr(*prPayload.CurrentPR)
	}
	for _, pr := range prPayload.ViewerCreated {
		fmt.Printf("Your Prs\n")
		printPr(pr)
	}
	for _, pr := range prPayload.ReviewRequested {
		fmt.Printf("Prs you need to review\n")
		printPr(pr)
	}
}

func printPr(pr graphql.PullRequest) {
	fmt.Printf("%d %s [%s]\n", pr.Number, pr.Title, pr.HeadRefName)
}
