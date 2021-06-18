package cmdutil

import (
	"os"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/spf13/cobra"
)

func EnableRepoOverride(cmd *cobra.Command, f *Factory) {
	cmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `[HOST/]OWNER/REPO` format")

	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		repoOverride, _ := cmd.Flags().GetString("repo")
		f.BaseRepo = OverrideBaseRepoFunc(f, repoOverride)
	}
}

func OverrideBaseRepoFunc(f *Factory, override string) func() (ghrepo.Interface, error) {
	if override == "" {
		override = os.Getenv("GH_REPO")
	}
	if override != "" {
		return func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName(override)
		}
	}
	return f.BaseRepo
}
