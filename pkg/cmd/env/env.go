package env

import (
	"fmt"
	"os"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdEnv(f *cmdutil.Factory) *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Display environment variables for gh",
		Long:  "Display values for gh environment variables.",
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			envVars := map[string]string{
				"GITHUB_TOKEN":            "an authentication token for github.com API requests. Setting this avoids\nbeing prompted to authenticate and takes precedence over previously stored credentials.",
				"GITHUB_ENTERPRISE_TOKEN": "an authentication token for API requests to GitHub Enterprise.",
				"GH_REPO":                 "specify the GitHub repository in the '[HOST/]OWNER/REPO' format for commands\nthat otherwise operate on a local repository.",
				"GH_HOST":                 "specify the GitHub hostname for commands that would otherwise assume\nthe 'github.com' host when not in a context of an existing repository.",
				"GH_EDITOR":               "the editor tool to use for authoring text (1st precedence).",
				"GIT_EDITOR":              "the editor tool to use for authoring text (2nd precedence).",
				"VISUAL":                  "the editor tool to use for authoring text (3rd precedence).",
				"EDITOR":                  "the editor tool to use for authoring text (4th precedence).",
				"BROWSER":                 "the web browser to use for opening links.",
				"DEBUG":                   "set to any value to enable verbose output to standard error. Include values 'api'\nor 'oauth' to print detailed information about HTTP requests or authentication flow.",
				"GLAMOR_STYLE":            "the style to use for rendering Markdown. See\nhttps://github.com/charmbracelet/glamour#styles",
				"NO_COLOR":                "avoid printing ANSI escape sequences for color output.",
			}

			orderedKeys := []string{
				"GITHUB_TOKEN",
				"GITHUB_ENTERPRISE_TOKEN",
				"GH_REPO",
				"GH_HOST",
				"GH_EDITOR",
				"GIT_EDITOR",
				"VISUAL",
				"EDITOR",
				"BROWSER",
				"DEBUG",
				"GLAMOR_STYLE",
				"NO_COLOR",
			}

			for _, k := range orderedKeys {
				if verbose {
					fmt.Fprintf(f.IOStreams.Out, "%s - %s\n", k, envVars[k])
				}

				fmt.Fprintf(f.IOStreams.Out, "%s: %s\n", k, os.Getenv(k))

				if verbose {
					fmt.Fprintln(f.IOStreams.Out, "")
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show environment variable descriptions")

	cmdutil.DisableAuthCheck(cmd)

	return cmd
}
