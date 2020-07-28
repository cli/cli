package repo

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
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
