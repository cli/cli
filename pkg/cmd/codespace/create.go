package codespace

import (
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

func newCreateCmd(app *codespaces.App) *cobra.Command {
	var opts codespaces.CreateOptions

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Create(cmd.Context(), opts)
		},
	}

	createCmd.Flags().StringVarP(&opts.Repo, "repo", "r", "", "repository name with owner: user/repo")
	createCmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "repository branch")
	createCmd.Flags().StringVarP(&opts.Machine, "machine", "m", "", "hardware specifications for the VM")
	createCmd.Flags().BoolVarP(&opts.ShowStatus, "status", "s", false, "show status of post-create command and dotfiles")

	return createCmd
}
