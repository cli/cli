package root

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

func installAccountOverride(cmd *cobra.Command, cfg gh.Config, f *cmdutil.Factory) {
	if previous := cmd.PersistentPreRunE; previous != nil {
		cmd.PersistentPreRunE = func(leaf *cobra.Command, args []string) error {
			if err := applyAccountOverride(leaf, args, cfg, f); err != nil {
				return err
			}
			return previous(leaf, args)
		}
	}
	for _, child := range cmd.Commands() {
		installAccountOverride(child, cfg, f)
	}
}

func applyAccountOverride(cmd *cobra.Command, args []string, cfg gh.Config, f *cmdutil.Factory) error {
	flag := cmd.Flags().Lookup("account")
	if flag == nil || flag != cmd.Root().PersistentFlags().Lookup("account") || !flag.Changed {
		return nil
	}
	account := flag.Value.String()
	if account == "" {
		return cmdutil.FlagErrorf("--account requires a saved account login")
	}
	// williammartin-human-review-note
	// It's pretty gross that we have to look up "auth" by name.
	// It really couples this logic to the command structure rather than having a more explicit way to identify the auth command.
	if isAuthCommand(cmd) {
		return cmdutil.FlagErrorf("--account selects authentication for commands such as 'gh pr create --account <login>', not auth management; use 'gh auth use <identity>' for a persistent choice or see this auth command's --help")
	}
	host, err := accountHostname(cmd, args, cfg, f)
	if err != nil {
		return err
	}
	// williammartin-human-review-note
	// Understanding if a user is available on the targeted host, or canonical host seems like it should
	// be abstracted over somewhere else rather than having this logic inline here.
	if canonical, ok := cfg.Authentication().HostForAPIHost(host); ok {
		host = canonical
	}
	found := false
	for _, user := range cfg.Authentication().UsersForHost(host) {
		if strings.EqualFold(user, account) {
			account, found = user, true
			break
		}
	}
	if !found {
		return fmt.Errorf("account %q is not saved for %s; run 'gh auth login --hostname %s'", account, host, host)
	}
	if _, err := identity.CredentialForAccount(cfg.Authentication(), host, account); err != nil {
		return err
	}
	// williammartin-human-review-note
	// I wonder whether rather than overriding an account, we should swap out the entire AuthConfig
	// to something static. That seems like it would help push conditionals up the stack.
	cfg.Authentication().SetAccountOverride(host, account)
	return nil
}

func isAuthCommand(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Name() == "auth" {
			return true
		}
	}
	return false
}

func accountHostname(cmd *cobra.Command, args []string, cfg gh.Config, f *cmdutil.Factory) (string, error) {
	host, _ := cfg.Authentication().DefaultHost()
	parent := ""
	if cmd.Parent() != nil {
		parent = cmd.Parent().Name()
	}
	// williammartin-human-review-note
	// This is absolutely terrible. It has to undersatnd the structure of every command and how it relates to repositories and resources.
	// What's happening here is that the host needs to be determined from command context, but the host can be determined in many ways,
	// such as env var, command-line flags, repository remotes, and more. I'd honestly be a bit shocked if this logic is correct in all cases.
	repositoryArgument := strings.Contains(cmd.Use, "<repository>") ||
		parent == "repo" && (cmd.Name() == "create" || cmd.Name() == "sync")
	resourceURL := cmd.Name() == "api" || parent == "pr" || parent == "issue" || parent == "gist"
	if len(args) > 0 && (repositoryArgument || resourceURL) && git.IsURL(args[0]) {
		u, err := git.ParseURL(args[0])
		if err != nil {
			return "", err
		}
		return strings.TrimPrefix(ghauth.NormalizeHostname(u.Host), "gist."), nil
	}
	if flag := cmd.Flags().Lookup("hostname"); flag != nil {
		if value := flag.Value.String(); value != "" {
			return value, nil
		}
		return host, nil
	}
	if len(args) > 0 && repositoryArgument {
		repo, err := ghrepo.FromFullNameWithHost(args[0], host)
		if err == nil {
			return repo.RepoHost(), nil
		}
		if cmd.Name() == "create" || cmd.Name() == "clone" {
			return host, nil
		}
	}
	repositoryContext := parent == "repo" && strings.Contains(cmd.Use, "repository")
	if flag := cmd.Flags().Lookup("repo"); flag != nil && flag.Value.Type() == "string" {
		repositoryContext = true
		value := flag.Value.String()
		if value == "" {
			value = os.Getenv("GH_REPO")
		}
		if value != "" {
			repo, err := ghrepo.FromFullNameWithHost(value, host)
			if err != nil {
				return "", err
			}
			return repo.RepoHost(), nil
		}
	}
	if parent == "repo" && cmd.Name() == "sync" {
		if source := cmd.Flags().Lookup("source"); source != nil && source.Value.String() != "" {
			repo, err := ghrepo.FromFullNameWithHost(source.Value.String(), host)
			if err != nil {
				return "", err
			}
			return repo.RepoHost(), nil
		}
	}
	if parent == "skill" && cmd.Name() == "publish" {
		client := f.GitClient.Copy()
		if len(args) > 0 {
			client.RepoDir = args[0]
		}
		remotes, err := client.Remotes(context.Background())
		if err == nil && len(remotes) > 0 && remotes[0].FetchURL != nil {
			return remotes[0].FetchURL.Host, nil
		}
	}
	if repositoryContext {
		remotes, err := f.Remotes()
		if err == nil && len(remotes) > 0 {
			return remotes[0].RepoHost(), nil
		}
	}
	return host, nil
}
