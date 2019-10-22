package command

import (
	//	"github.com/github/gh-cli/api"
	//	"github.com/github/gh-cli/context"
	"fmt"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
	"os"
	"runtime"
)

var draftF bool
var titleF string
var bodyF string
var baseF string

func prCreate() error {
	ucc, err := git.UncommittedChangeCount()
	if err != nil {
		return fmt.Errorf(
			"could not determine state of working directory: %v", err)
	}
	if ucc > 0 {
		noun := "change"
		if ucc > 1 {
			noun = "changes"
		}

		fmt.Printf(
			"%s %d uncommitted %s %s",
			utils.Red("!!!"),
			ucc,
			noun,
			utils.Red("!!!"))
	}

	return nil
}

func determineEditor() string {
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "nano"
}

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
