package ghcs

import (
	"github.com/spf13/cobra"
)

var version = "DEV" // Replaced in the release build process (by GoReleaser or Homebrew) by the git tag version number.

func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "ghcs",
		SilenceUsage:  true,  // don't print usage message after each error (see #80)
		SilenceErrors: false, // print errors automatically so that main need not
		Long: `Unofficial CLI tool to manage GitHub Codespaces.

Running commands requires the GITHUB_TOKEN environment variable to be set to a
token to access the GitHub API with.`,
		Version: version,
	}

	root.AddCommand(newCodeCmd(app))
	root.AddCommand(newCreateCmd(app))
	root.AddCommand(newDeleteCmd(app))
	root.AddCommand(newListCmd(app))
	root.AddCommand(newLogsCmd(app))
	root.AddCommand(newPortsCmd(app))
	root.AddCommand(newSSHCmd(app))

	return root
}
