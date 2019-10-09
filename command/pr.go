package command

import (
	"fmt"

	"github.com/github/gh-cli/api"
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
	prPayload, err := api.PullRequests()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Current Pr\n")
	if prPayload.CurrentPR != nil {
		printPr(*prPayload.CurrentPR)
	}
	fmt.Printf("Your Prs\n")
	for _, pr := range prPayload.ViewerCreated {
		printPr(pr)
	}
	fmt.Printf("Prs you need to review\n")
	for _, pr := range prPayload.ReviewRequested {
		printPr(pr)
	}
}

func printPr(pr api.PullRequest) {
	fmt.Printf("  #%d %s [%s]\n", pr.Number, pr.Title, pr.HeadRefName)
}
