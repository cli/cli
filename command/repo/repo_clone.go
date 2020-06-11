package repo

import (
	"os"
	"path"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/command"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/spf13/cobra"
)

func RepoCloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone <repository> [<directory>]",
		Short: "Clone a repository locally",
		Long: `Clone a GitHub repository locally.

If the "OWNER/" portion of the "OWNER/REPO" repository argument is omitted, it
defaults to the name of the authenticating user.

To pass 'git clone' flags, separate them with '--'.`,
		Args: cobra.MinimumNArgs(1),
		RunE: repoClone,
	}

	return cmd
}

func repoClone(cmd *cobra.Command, args []string) error {
	ctx := command.ContextForCommand(cmd)
	apiClient, err := command.ApiClientForContext(ctx)
	if err != nil {
		return nil
	}

	cloneURL := args[0]
	if !strings.Contains(cloneURL, ":") {
		if !strings.Contains(cloneURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient)
			if err != nil {
				return err
			}
			cloneURL = currentUser + "/" + cloneURL
		}
		cloneURL = command.FormatRemoteURL(cmd, cloneURL)
	}

	var repo, parentRepo ghrepo.Interface

	// TODO: consider caching and reusing `git.ParseSSHConfig().Translator()`
	// here to handle hostname aliases in SSH remotes
	if u, err := git.ParseURL(cloneURL); err == nil {
		repo, _ = ghrepo.FromURL(u)
	}
	if repo != nil {
		parentRepo, err = api.RepoParent(apiClient, repo)
		if err != nil {
			return nil
		}
	}

	cloneDir, err := runClone(cloneURL, args[1:])
	if err != nil {
		return err
	}
	if parentRepo != nil {
		err := addUpstreamRemote(cmd, parentRepo, cloneDir)
		if err != nil {
			return err
		}
	}

	return nil
}

func runClone(cloneURL string, args []string) (target string, err error) {
	cloneArgs, target := parseCloneArgs(args)
	cloneArgs = append(cloneArgs, cloneURL)

	// If the args contain an explicit target, pass it to clone
	// otherwise, parse the URL to determine where git cloned it to so we can return it
	if target != "" {
		cloneArgs = append(cloneArgs, target)
	} else {
		target = path.Base(strings.TrimSuffix(cloneURL, ".git"))
	}

	cloneArgs = append([]string{"clone"}, cloneArgs...)

	cloneCmd := git.GitCommand(cloneArgs...)
	cloneCmd.Stdin = os.Stdin
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr

	err = run.PrepareCmd(cloneCmd).Run()
	return
}
