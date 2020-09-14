package root

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewHelpTopic(topic string) *cobra.Command {
	return &cobra.Command{
		Use: "environment",
		Long: heredoc.Doc(`
			GITHUB_TOKEN: an authentication token for github.com API requests. Setting this avoids
			being prompted to authenticate and takes precedence over previously stored credentials.

			GITHUB_ENTERPRISE_TOKEN: an authentication token for API requests to GitHub Enterprise.

			GH_REPO: specify the GitHub repository in the "[HOST/]OWNER/REPO" format for commands
			that otherwise operate on a local repository.

			GH_HOST: specify the GitHub hostname for commands that would otherwise assume
			the "github.com" host when not in a context of an existing repository.

			GH_EDITOR, GIT_EDITOR, VISUAL, EDITOR (in order of precedence): the editor tool to use
			for authoring text.

			BROWSER: the web browser to use for opening links.

			DEBUG: set to any value to enable verbose output to standard error. Include values "api"
			or "oauth" to print detailed information about HTTP requests or authentication flow.

			GLAMOUR_STYLE: the style to use for rendering Markdown. See
			https://github.com/charmbracelet/glamour#styles

			NO_COLOR: avoid printing ANSI escape sequences for color output.
		`),
		Hidden: true,
		Annotations: map[string]string{
			"helpTopic": "true",
		},
	}
}
