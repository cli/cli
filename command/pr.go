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
	fmt.Printf("%+v!\n", prPayload)
}
