package authmap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type mapScope struct {
	hostname string
	owner    string
	repo     string
	ownerAll bool
}

type repositoryMapper interface {
	SetUserForOwner(hostname, owner, user string) error
	SetUserForRepository(hostname, owner, repo, user string) error
	DeleteUserForOwner(hostname, owner string) error
	DeleteUserForRepository(hostname, owner, repo string) error
	RepositoryUserMappings(hostname string) config.RepositoryUserMappings
}

type SetOptions struct {
	IO       *iostreams.IOStreams
	Config   func() (gh.Config, error)
	Scope    string
	Username string
	Hostname string
}

type ListOptions struct {
	IO       *iostreams.IOStreams
	Config   func() (gh.Config, error)
	Hostname string
}

type RemoveOptions struct {
	IO       *iostreams.IOStreams
	Config   func() (gh.Config, error)
	Scope    string
	Hostname string
}

func NewCmdMap(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "map <command>",
		Short: "Manage repository account mappings",
		Long: heredoc.Docf(`
			Manage mappings that select a specific authenticated account for repository scopes.

			Scopes can be:
			- %[1]sOWNER/*%[1]s
			- %[1]sOWNER/REPO%[1]s
			- %[1]sHOST/OWNER/*%[1]s
			- %[1]sHOST/OWNER/REPO%[1]s

			Mappings are applied for API requests when a repository context is available.
		`, "`"),
	}

	cmd.AddCommand(newCmdSet(f))
	cmd.AddCommand(newCmdList(f))
	cmd.AddCommand(newCmdRemove(f))

	return cmd
}

func newCmdSet(f *cmdutil.Factory) *cobra.Command {
	opts := &SetOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "set <scope>",
		Args:  cobra.ExactArgs(1),
		Short: "Set a repository account mapping",
		Example: heredoc.Doc(`
			# Use account 'work-user' for all repos under Devolutions on github.com
			$ gh auth map set Devolutions/* --user work-user

			# Use account 'personal-user' for one specific repository
			$ gh auth map set github.com/awakecoding/MsRdpEx --user personal-user
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Scope = args[0]
			return setRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Username, "user", "u", "", "Account username to associate with this scope")
	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "Hostname to apply when scope omits host")
	_ = cmd.MarkFlagRequired("user")

	return cmd
}

func newCmdList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Args:  cobra.ExactArgs(0),
		Short: "List repository account mappings",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "Show mappings for a single hostname")

	return cmd
}

func newCmdRemove(f *cmdutil.Factory) *cobra.Command {
	opts := &RemoveOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "remove <scope>",
		Args:  cobra.ExactArgs(1),
		Short: "Remove a repository account mapping",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Scope = args[0]
			return removeRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "", "Hostname to apply when scope omits host")

	return cmd
}

func setRun(opts *SetOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	defaultHost, _ := authCfg.DefaultHost()
	scope, err := parseScope(opts.Scope, opts.Hostname, defaultHost)
	if err != nil {
		return err
	}

	knownUsers := authCfg.UsersForHost(scope.hostname)
	if !slicesContainsFold(knownUsers, opts.Username) {
		return fmt.Errorf("not logged in to %s account %s", scope.hostname, opts.Username)
	}

	mapper, ok := authCfg.(repositoryMapper)
	if !ok {
		return fmt.Errorf("repository mappings are not supported by this configuration")
	}

	if scope.ownerAll {
		err = mapper.SetUserForOwner(scope.hostname, scope.owner, opts.Username)
	} else {
		err = mapper.SetUserForRepository(scope.hostname, scope.owner, scope.repo, opts.Username)
	}
	if err != nil {
		return err
	}

	if err := cfg.Write(); err != nil {
		return fmt.Errorf("failed to write config to disk: %w", err)
	}

	if scope.ownerAll {
		fmt.Fprintf(opts.IO.Out, "%s/%s/* => %s\n", scope.hostname, scope.owner, opts.Username)
	} else {
		fmt.Fprintf(opts.IO.Out, "%s/%s/%s => %s\n", scope.hostname, scope.owner, scope.repo, opts.Username)
	}

	return nil
}

func listRun(opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	mapper, ok := authCfg.(repositoryMapper)
	if !ok {
		return fmt.Errorf("repository mappings are not supported by this configuration")
	}

	hosts := []string{}
	if opts.Hostname != "" {
		hosts = []string{strings.ToLower(strings.TrimSpace(opts.Hostname))}
	} else {
		hosts = authCfg.Hosts()
		sort.Strings(hosts)
	}

	printed := false
	for _, host := range hosts {
		mappings := mapper.RepositoryUserMappings(host)
		if len(mappings.Owners) == 0 && len(mappings.Repos) == 0 {
			continue
		}

		fmt.Fprintln(opts.IO.Out, host)

		owners := make([]string, 0, len(mappings.Owners))
		for owner := range mappings.Owners {
			owners = append(owners, owner)
		}
		sort.Strings(owners)
		for _, owner := range owners {
			fmt.Fprintf(opts.IO.Out, "  %s/* => %s\n", owner, mappings.Owners[owner])
		}

		repoOwners := make([]string, 0, len(mappings.Repos))
		for owner := range mappings.Repos {
			repoOwners = append(repoOwners, owner)
		}
		sort.Strings(repoOwners)
		for _, owner := range repoOwners {
			repos := make([]string, 0, len(mappings.Repos[owner]))
			for repo := range mappings.Repos[owner] {
				repos = append(repos, repo)
			}
			sort.Strings(repos)
			for _, repo := range repos {
				fmt.Fprintf(opts.IO.Out, "  %s/%s => %s\n", owner, repo, mappings.Repos[owner][repo])
			}
		}

		printed = true
	}

	if !printed {
		fmt.Fprintln(opts.IO.Out, "No repository account mappings configured.")
	}

	return nil
}

func removeRun(opts *RemoveOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	defaultHost, _ := authCfg.DefaultHost()
	scope, err := parseScope(opts.Scope, opts.Hostname, defaultHost)
	if err != nil {
		return err
	}

	mapper, ok := authCfg.(repositoryMapper)
	if !ok {
		return fmt.Errorf("repository mappings are not supported by this configuration")
	}

	if scope.ownerAll {
		err = mapper.DeleteUserForOwner(scope.hostname, scope.owner)
	} else {
		err = mapper.DeleteUserForRepository(scope.hostname, scope.owner, scope.repo)
	}
	if err != nil {
		return err
	}

	if err := cfg.Write(); err != nil {
		return fmt.Errorf("failed to write config to disk: %w", err)
	}

	return nil
}

func parseScope(scope, hostnameFlag, defaultHost string) (mapScope, error) {
	hostnameFlag = strings.ToLower(strings.TrimSpace(hostnameFlag))
	defaultHost = strings.ToLower(strings.TrimSpace(defaultHost))
	scope = strings.TrimSpace(scope)

	if scope == "" {
		return mapScope{}, fmt.Errorf("scope cannot be blank")
	}

	parts := strings.Split(scope, "/")
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return mapScope{}, fmt.Errorf("invalid scope %q", scope)
		}
	}

	var out mapScope
	switch len(parts) {
	case 2:
		out.hostname = hostnameFlag
		if out.hostname == "" {
			out.hostname = defaultHost
		}
		out.owner = strings.ToLower(parts[0])
		if parts[1] == "*" {
			out.ownerAll = true
		} else {
			out.repo = strings.ToLower(parts[1])
		}
	case 3:
		out.hostname = strings.ToLower(parts[0])
		if hostnameFlag != "" && hostnameFlag != out.hostname {
			return mapScope{}, fmt.Errorf("scope host %q conflicts with --hostname %q", out.hostname, hostnameFlag)
		}
		out.owner = strings.ToLower(parts[1])
		if parts[2] == "*" {
			out.ownerAll = true
		} else {
			out.repo = strings.ToLower(parts[2])
		}
	default:
		return mapScope{}, fmt.Errorf("invalid scope %q: expected OWNER/*, OWNER/REPO, HOST/OWNER/*, or HOST/OWNER/REPO", scope)
	}

	if out.hostname == "" {
		return mapScope{}, fmt.Errorf("unable to determine hostname; pass --hostname or include HOST in scope")
	}

	if !out.ownerAll && out.repo == "" {
		return mapScope{}, fmt.Errorf("invalid scope %q", scope)
	}

	return out, nil
}

func slicesContainsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
