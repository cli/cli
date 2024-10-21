package cmdutil

import (
	ghContext "github.com/cli/cli/v2/context"
	"os"
	"sort"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
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

func PromptForRepo(baseRepo ghrepo.Interface, remotes func() (ghContext.Remotes, error), survey prompter.Prompter) (ghrepo.Interface, error) {
	var defaultRepo string
	var remoteArray []string

	if remotes, _ := remotes(); remotes != nil {
		if defaultRemote, _ := remotes.ResolvedRemote(); defaultRemote != nil {
			// this is a remote explicitly chosen via `repo set-default`
			defaultRepo = ghrepo.FullName(defaultRemote)
		} else if len(remotes) > 0 {
			// as a fallback, just pick the first remote
			defaultRepo = ghrepo.FullName(remotes[0])
		}

		for _, remote := range remotes {
			remoteArray = append(remoteArray, ghrepo.FullName(remote))
		}
	}

	baseRepoInput, errInput := survey.Select("Select a base repo", defaultRepo, remoteArray)
	if errInput != nil {
		return baseRepo, errInput
	}

	selectedRepo, errSelectedRepo := ghrepo.FromFullName(remoteArray[baseRepoInput])
	if errSelectedRepo != nil {
		return baseRepo, errSelectedRepo
	}

	return selectedRepo, nil
}
