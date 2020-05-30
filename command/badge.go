package command

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func init() {
	RootCmd.AddCommand(badgeCmd)

	badgeCmd.Flags().BoolP("no-link", "n", false, "Don't wrap badge in link to action")
	badgeCmd.Flags().BoolP("output", "o", false, "Don't copy badge to clipboard, just output it")
}

var badgeCmd = &cobra.Command{
	Use:   "badge [GitHub username] [Repository name] [Action file name]",
	Short: "Generate a README badge for a GitHub action",
	Long: `Generate a markdown viewable badge for the given GitHub action.

Examples:

  gh badge twbs bootstrap test.yml
  gh badge cli cli lint.yml
`,
	Args: cobra.MaximumNArgs(3),
	RunE: badge,
}

func badge(cmd *cobra.Command, args []string) error {
	username := args[0]
	repository := args[1]
	fileName := args[2]

	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	var yamlContents map[string]string
	err = yaml.Unmarshal(bytes, &yamlContents)
	if err != nil {
		return err
	}
	actionName := yamlContents["name"]

	var badge string
	badge = fmt.Sprintf("![%v](https://github.com/%v/%v/workflows/%v/badge.svg)", actionName, username, repository, strings.ReplaceAll(actionName, " ", "%20"))

	link, err := cmd.Flags().GetBool("no-link")
	if err != nil {
		return err
	}
	if link {
		badge = fmt.Sprintf(`[%v](https://github.com/%v/%v/actions?query=workflow%%3A"%v")`, badge, username, repository, strings.ReplaceAll(actionName, " ", "+"))
	}

	output, err := cmd.Flags().GetBool("output")
	if err != nil {
		return err
	}

	if output {
		fmt.Println(badge)
	} else {
		err := clipboard.WriteAll(badge)
		if err != nil {
			return err
		}
		fmt.Println("Badge copied to clipboard!")
	}

	return nil
}
