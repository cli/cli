package command

import (
	"fmt"
	"strings"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoViewCmd)
}

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "View repositories",
	Long: `Work with GitHub repositories.

A repository can be supplied as an argument in any of the following formats:
- by owner/repo, e.g. "cli/cli"
- by URL, e.g. "https://github.com/cli/cli"`,
}

var repoViewCmd = &cobra.Command{
	Use:   "view [{<owner/repo> | <url>}]",
	Short: "View a repository in the browser",
	Long: `View a repository specified by the argument in the browser.

Without an argument, the repository that belongs to the current
branch is opened.`,
	RunE: repoView,
}

func repoView(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return err
	}

	var openURL string
	if len(args) == 0 {
		openURL = fmt.Sprintf("https://github.com/%s", ghrepo.FullName(*baseRepo))
	} else {
		if strings.HasPrefix(args[0], "http") {
			openURL = args[0]
		} else {
			openURL = fmt.Sprintf("https://github.com/%s", args[0])
		}
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Opening %s in your browser.\n", openURL)
	return utils.OpenInBrowser(openURL)
}
