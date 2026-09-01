package telemetry

import (
	"os"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

// GuessTargetHost determines the GitHub host that the current command targets.
// It checks sources in priority order: explicit flags/args first (--repo flag,
// --hostname flag, repo positional arg), then environment (GH_REPO), then
// git remotes, and finally the configured default host.
//
// It returns the empty string when no signal can be found. Callers that need a
// categorical bucket should pipe the result through ghinstance.CategorizeHost,
// which maps "" to "uncategorized".
//
// The baseRepo and config parameters correspond to cmdutil.Factory's BaseRepo
// and Config fields. They are passed as narrow functions instead of the full
// Factory to avoid an import cycle between this package and pkg/cmdutil.
func GuessTargetHost(
	cmd *cobra.Command,
	baseRepo func() (ghrepo.Interface, error),
	config func() (gh.Config, error),
) string {
	// 1. --repo flag (explicit flag takes highest priority)
	if repoFlag, err := cmd.Flags().GetString("repo"); err == nil && repoFlag != "" {
		if r, err := ghrepo.FromFullName(repoFlag); err == nil {
			return r.RepoHost()
		}
	}

	// 2. --hostname flag (used by auth, api, attestation commands)
	if hostname, err := cmd.Flags().GetString("hostname"); err == nil && hostname != "" {
		return ghauth.NormalizeHostname(hostname)
	}

	// 3. Positional repo argument for "gh repo <subcommand> [OWNER/REPO]" commands
	if cmd.Parent() != nil && cmd.Parent().Name() == "repo" {
		if args := cmd.Flags().Args(); len(args) > 0 {
			if r, err := ghrepo.FromFullName(args[0]); err == nil {
				return r.RepoHost()
			}
		}
	}

	// 4. GH_REPO env var
	if ghRepo := os.Getenv("GH_REPO"); ghRepo != "" {
		if r, err := ghrepo.FromFullName(ghRepo); err == nil {
			return r.RepoHost()
		}
	}

	// 5. Git remotes; expected to be BaseRepoFunc (local, no network) in root's closure.
	// Only attempt this for commands that are likely repo-scoped (have a --repo flag)
	// to avoid triggering git remote discovery for host-agnostic commands like "gh version".
	if baseRepo != nil && cmd.Flags().Lookup("repo") != nil {
		if repo, err := baseRepo(); err == nil {
			return repo.RepoHost()
		}
	}

	// 6. Default host from config / GH_HOST env
	if config != nil {
		if cfg, err := config(); err == nil {
			host, _ := cfg.Authentication().DefaultHost()
			return host
		}
	}

	return ""
}
