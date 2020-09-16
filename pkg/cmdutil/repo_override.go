package cmdutil

import (
	"os"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/spf13/cobra"
)

func EnableRepoOverride(cmd *cobra.Command, f *Factory) {
	cmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")

	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		repoOverride, _ := cmd.Flags().GetString("repo")
		if repoFromEnv := os.Getenv("GH_REPO"); repoOverride == "" && repoFromEnv != "" {
			repoOverride = repoFromEnv
		}
		if repoOverride != "" {
			// NOTE: this mutates the factory
			f.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName(repoOverride)
			}
		}
	}
}
