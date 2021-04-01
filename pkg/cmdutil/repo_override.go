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
		if repoFromEnv := os.Getenv("GH_REPO"); repoOverride == "" && repoFromEnv != "" {
			repoOverride = repoFromEnv
		}
		if repoOverride != "" {
			// NOTE: this mutates the factory
			f.BaseRepo = OverrideBaseRepoFunc(f, repoOverride)
		}
	}
}

func OverrideBaseRepoFunc(f *Factory, repoOverride string) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		fb := func() (string, error) {
			cfg, err := f.Config()
			if err != nil {
				return "", err
			}
			return cfg.DefaultHost()
		}
		return ghrepo.FromName(repoOverride, fb, nil)
	}
}
