package command

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func init() {
	repoCmd.AddCommand(repoCreditsCmd)
	repoCreditsCmd.Flags().BoolP("static", "s", false, "Print a static version of the credits")
}

var repoCmd = &cobra.Command{
	Use:   "repo <command>",
	Short: "Create, clone, fork, and view repositories",
	Long:  `Work with GitHub repositories`,
	Example: heredoc.Doc(`
	$ gh repo create
	$ gh repo clone cli/cli
	$ gh repo view --web
	`),
	Annotations: map[string]string{
		"IsCore": "true",
		"help:arguments": `
A repository can be supplied as an argument in any of the following formats:
- "OWNER/REPO"
- by URL, e.g. "https://github.com/OWNER/REPO"`},
}

var repoCreditsCmd = &cobra.Command{
	Use:   "credits [<repository>]",
	Short: "View credits for a repository",
	Example: heredoc.Doc(`
     # view credits for the current repository
     $ gh repo credits

     # view credits for a specific repository
     $ gh repo credits cool/repo

     # print a non-animated thank you
     $ gh repo credits -s

     # pipe to just print the contributors, one per line
     $ gh repo credits | cat
  `),
	Args:   cobra.MaximumNArgs(1),
	RunE:   repoCredits,
	Hidden: true,
}

func repoCredits(cmd *cobra.Command, args []string) error {
	return credits(cmd, args)
}
