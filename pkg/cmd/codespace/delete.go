package codespace

import (
	"errors"
	"time"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

func newDeleteCmd(app *codespaces.App) *cobra.Command {
	opts := codespaces.DeleteOptions{
		Now: time.Now,
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.DeleteAll && opts.RepoFilter != "" {
				return errors.New("both --all and --repo is not supported")
			}
			return app.Delete(cmd.Context(), opts)
		},
	}

	deleteCmd.Flags().StringVarP(&opts.CodespaceName, "codespace", "c", "", "Name of the codespace")
	deleteCmd.Flags().BoolVar(&opts.DeleteAll, "all", false, "Delete all codespaces")
	deleteCmd.Flags().StringVarP(&opts.RepoFilter, "repo", "r", "", "Delete codespaces for a `repository`")
	deleteCmd.Flags().BoolVarP(&opts.SkipConfirm, "force", "f", false, "Skip confirmation for codespaces that contain unsaved changes")
	deleteCmd.Flags().Uint16Var(&opts.KeepDays, "days", 0, "Delete codespaces older than `N` days")

	return deleteCmd
}
