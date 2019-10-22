package command

import (
	//	"github.com/github/gh-cli/api"
	//	"github.com/github/gh-cli/context"
	"fmt"
	"github.com/spf13/cobra"
)

var draftF bool
var titleF string
var bodyF string
var baseF string

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pull request",
	RunE: func(cmd *cobra.Command, args []string) error {
		return prCreate()
	},
}

func init() {
	prCreateCmd.Flags().BoolVarP(&draftF, "draft", "d", false,
		"Mark PR as a draft")
	prCreateCmd.Flags().StringVarP(&titleF, "title", "t", "",
		"Supply a title. Will prompt for one otherwise.")
	prCreateCmd.Flags().StringVarP(&bodyF, "body", "b", "",
		"Supply a body. Will prompt for one otherwise.")
	prCreateCmd.Flags().StringVarP(&baseF, "base", "T", "",
		"The target branch you want your PR merged into in the format remote:branch.")
}

func prCreate() error {
	fmt.Println("stubs")
	return nil
}
