package cmdutil

import (
	"os"
	"sort"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/spf13/cobra"
)

// executeParentHook re-runs the nearest ancestor's persistent pre-run hook,
// which the hook installed by EnableRepoOverride would otherwise shadow. By
// default cobra runs only the nearest PersistentPreRunE found walking up from
// the invoked command, so without this the nearest ancestor hook, such as the
// root auth gate, would never run for a repo-override command.
//
// That ancestor hook receives the invoked leaf cmd, not the ancestor, matching
// how cobra passes the leaf to every persistent hook:
// https://github.com/spf13/cobra/blob/v1.10.2/command.go#L984-L986
//
// cobra's EnableTraverseRunHooks global is the native equivalent and runs the
// whole root-to-leaf chain for us, but it is global. Enabling it would change
// pre-run behavior for every command: double-running the parents that issue
// develop and EnableRepoOverride re-run by hand, and un-suppressing the root
// gate that agent-task and skills intentionally shadow.
func executeParentHook(overrideCmd, cmd *cobra.Command, args []string) error {
	for p := overrideCmd.Parent(); p != nil; p = p.Parent() {
		if p.PersistentPreRunE != nil {
			return p.PersistentPreRunE(cmd, args)
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

	overrideCmd := cmd
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := executeParentHook(overrideCmd, cmd, args); err != nil {
			return err
		}
		repoOverride, _ := cmd.Flags().GetString("repo")
		f.BaseRepo = OverrideBaseRepoFunc(f.BaseRepo, repoOverride)
		return nil
	}
}

func OverrideBaseRepoFunc(baseRepoFunc func() (ghrepo.Interface, error), override string) func() (ghrepo.Interface, error) {
	if override == "" {
		override = os.Getenv("GH_REPO")
	}
	if override != "" {
		return func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName(override)
		}
	}
	return baseRepoFunc
}
