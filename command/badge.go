package command

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(badgeCmd)

	badgeCmd.Flags().BoolP("link", "l", true, "Badge wrapped in link to badge")
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
	RunE: credits,
}
