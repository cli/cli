package cmdutil

import (
	"os"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/spf13/cobra"
)

func executeParentHooks(cmd *cobra.Command, args []string) error {
	for cmd.HasParent() {
		cmd = cmd.Parent()
		if cmd.PersistentPreRunE != nil {
			return cmd.PersistentPreRunE(cmd, args)
		}
	}
	return nil
}

func EnableRepoOverride(cmd *cobra.Command, f *Factory) {
	cmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `[HOST/]OWNER/REPO` format")

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := executeParentHooks(cmd, args); err != nil {
			return err
		}
		repoOverride, _ := cmd.Flags().GetString("repo")
		f.BaseRepo = OverrideBaseRepoFunc(f, repoOverride)
		return nil
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
