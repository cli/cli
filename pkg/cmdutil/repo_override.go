package cmdutil

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
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
	_ = cmd.RegisterFlagCompletionFunc("repo", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		remotes, err := f.Remotes()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		config, err := f.Config()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		defaultHost, _ := config.Authentication().DefaultHost()

		var results []string
		for _, remote := range remotes {
			repo := remote.RepoOwner() + "/" + remote.RepoName()
			if !strings.EqualFold(remote.RepoHost(), defaultHost) {
				repo = remote.RepoHost() + "/" + repo
			}
			if strings.HasPrefix(repo, toComplete) {
				results = append(results, repo)
			}
		}
		sort.Strings(results)
		return results, cobra.ShellCompDirectiveNoFileComp
	})

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := executeParentHooks(cmd, args); err != nil {
			return err
		}

		// First, get the value of the flag.
		// We can ignore errors here because the flag is guaranteed to exist and be string.
		userProvidedRepo, _ := cmd.Flags().GetString("repo")
		if userProvidedRepo == "" {
			// If there was no flag set, then check the GH_REPO environment variable,
			// which has lower precedence.
			userProvidedRepo = os.Getenv("GH_REPO")
			// If the environment variable was set, then set the value of the `repo` flag,
			// this ensures that checks for `HasChanged` work correctly. It's a bit "spooky"
			// action at a distance because the flag will not have been set by the user,
			// but perhaps it makes sense?
			//
			// The reason we need this is because there are `secret` commands that need to know whether
			// the user set the repo, and the alternative is them having knowledge of the flag and env var,
			// or by adjusting all our types so that the "source" of the BaseRepo is surfaced, which is a
			// pretty big change.
			if userProvidedRepo != "" {
				if err := cmd.Flags().Set("repo", userProvidedRepo); err != nil {
					return fmt.Errorf("failed to set repo override flag: %v", err)
				}
			}
		}

		// If the user provided the repo from either source, then we can return that
		// directly inside a new BaseRepo function.
		if userProvidedRepo != "" {
			f.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName(userProvidedRepo)
			}
		}

		return nil
	}
}

type baseRepoFn func() (ghrepo.Interface, error)

func PrioritiseEnvBaseRepoFunc(baseRepo baseRepoFn) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		repo := os.Getenv("GH_REPO")
		if repo == "" {
			return baseRepo()
		}
		return ghrepo.FromFullName(repo)
	}
}
