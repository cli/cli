package use

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// UseOptions holds the scope and identity chosen by auth use.
type UseOptions struct {
	Identity    string
	Directory   string
	Owner       string
	Host        string
	Unset       bool
	List        bool
	View        bool
	Explanation bool
	Exporter    cmdutil.Exporter
}

// NewCmdUse applies or removes an experimental persistent identity binding.
func NewCmdUse(f *cmdutil.Factory, runF func(*UseOptions) error) *cobra.Command {
	opts := &UseOptions{}
	cmd := &cobra.Command{
		Use: "use [identity]", Short: "Use an identity for this repository and its worktrees, or this directory tree (experimental)",
		Long: heredoc.Doc(`
			Inside any repository checkout, choose an identity for that repository's main checkout
			and all existing and future linked worktrees, wherever they are located.
			Outside a repository, choose an identity for this directory tree. Repositories whose
			main Git directory is inside that tree inherit it, including their external worktrees.
			A worktree placed here from a repository elsewhere does not inherit this directory choice.
			Repository choices beat directory choices. More specific directories beat broader ones,
			then owner choices provide a fallback. Unrelated repositories nested inside a checkout
			do not inherit its repository choice.
			--owner applies to repositories of that owner on the identity's host, in any directory.
			The saved active account is unchanged. Editing the identity updates every choice using it.

			Applying a choice writes your global Git config: an include of gh's auth.gitconfig,
			a host-wide HTTPS credential helper, and an SSH dispatcher used by Git in all directories.
			Keep this gh executable in place until all choices are removed. Compatible existing SSH
			commands are preserved; unsupported connection setups fail without replacing them.
			If a non-gh HTTPS helper is active for the host (including osxkeychain or Git Credential
			Manager), setup changes nothing. Existing gh helper resets are respected.
			Submodules, separate Git directories, and bare repositories are not
			supported for persistent identities. Do not bind a tree containing these repositories.
			Windows persistence is refused before changing Git. Git 2.36 or newer is required.
			Token environment variables (GH_TOKEN/GITHUB_TOKEN or their enterprise equivalents)
			block persistent identity use, including SSH. For a different gh command, use an
			explicit account, for example gh pr create --account alice; gh auth use does not accept --account.

			Use --view (or no arguments) for the selected identity as JSON, and --list for all choices.
			Use --explain to see why an identity applies, how gh and Git authenticate, the Git name/email,
			and which config files Git included. --json, --jq and --template require --explain.
			Authentication JSON statuses are declared (configured, not checked), blocked,
			overridden, or unmanaged. Attribution is observed inside a repository, declared for
			the selected identity's own defaults outside one, or unavailable with neither a
			repository nor a selected identity.

			--unset inside any worktree removes the repository's shared choice for all its worktrees.
			Outside a repository it removes the choice stored at this exact directory, not an
			ancestor's choice. Use --directory to target another checkout or stored directory.
			Inspection and --unset default to github.com, not GH_HOST or the repository remote.
			Removing the last choice also removes gh's managed global Git include.
		`),
		Example: heredoc.Doc(`
			$ gh auth use work
			$ gh auth use work --directory ~/src/work
			$ gh auth use work --owner company
			$ gh auth use --view
			$ gh auth use --explain --json selection --jq '.selection.identity.name'
			$ gh auth use --unset --directory ~/src/work
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Identity = args[0]
			}
			if opts.View && (opts.Identity != "" || opts.Unset || opts.List || opts.Explanation) {
				return cmdutil.FlagErrorf("--view cannot be combined with an identity, --unset, --list, or --explain")
			}
			if opts.Identity != "" && cmd.Flags().Changed("hostname") {
				return cmdutil.FlagErrorf("--hostname is for inspection or --unset; a named identity supplies its own hostname")
			}
			if cmd.Flags().Changed("owner") && opts.Owner == "" {
				return cmdutil.FlagErrorf("--owner cannot be empty")
			}
			if opts.Host == "" {
				return cmdutil.FlagErrorf("--hostname cannot be empty")
			}
			if opts.Owner != "" && opts.Directory != "" {
				return cmdutil.FlagErrorf("--owner cannot be combined with --directory")
			}
			if opts.Unset && opts.Identity != "" {
				return cmdutil.FlagErrorf("--unset does not take an identity")
			}
			if opts.Explanation && (opts.Identity != "" || opts.Unset || opts.List) {
				flag := "--explain"
				if cmd.Flags().Changed("explanation") {
					flag = "--explanation"
				}
				return cmdutil.FlagErrorf("%s cannot be combined with an identity, --unset, or --list", flag)
			}
			if opts.Exporter != nil && !opts.Explanation {
				return cmdutil.FlagErrorf("--json requires --explain")
			}
			if opts.List && (opts.Identity != "" || opts.Unset || opts.Owner != "" || opts.Directory != "") {
				return cmdutil.FlagErrorf("--list cannot be combined with a binding scope or identity")
			}
			if runF != nil {
				return runF(opts)
			}
			return useRun(f, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Directory, "directory", "", "Target directory or checkout; a checkout selects its whole repository family")
	cmd.Flags().StringVar(&opts.Owner, "owner", "", "Repository owner for selection (repository/directory choices still win), or the owner choice to apply/unset")
	cmd.Flags().StringVar(&opts.Host, "hostname", "github.com", "Host to inspect or unset; does not follow GH_HOST or remotes (a named identity supplies its host)")
	cmd.Flags().BoolVar(&opts.Unset, "unset", false, "Remove this repository-family, directory, or owner choice; fallback choices remain")
	cmd.Flags().BoolVar(&opts.List, "list", false, "Show all bindings as JSON")
	cmd.Flags().BoolVar(&opts.View, "view", false, "Show the selected identity as JSON (the default with no arguments)")
	cmd.Flags().BoolVar(&opts.Explanation, "explanation", false, "Explain the identity selected here and its effects on gh and Git")
	_ = cmd.Flags().MarkHidden("explanation")
	cmd.Flags().BoolVar(&opts.Explanation, "explain", false, "Show why the identity applies and the account, SSH, and Git settings here")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, []string{"context", "selection", "authentication", "attribution", "configuration", "warnings"})
	return cmd
}

func useRun(f *cmdutil.Factory, opts *UseOptions) error {
	opts.Host = strings.ToLower(opts.Host)
	s, err := identity.Open()
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(f.IOStreams.Out)
	encoder.SetIndent("", "  ")
	if opts.Explanation {
		executable := f.ExecutablePath
		if executable == "" {
			executable, err = os.Executable()
			if err != nil {
				return err
			}
		}
		result, err := s.ExplainReport(opts.Host, opts.Owner, opts.Directory, executable)
		if err != nil {
			return err
		}
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		result.CheckAccounts(cfg.Authentication())
		if opts.Exporter != nil {
			return opts.Exporter.Write(f.IOStreams, cmdutil.StructExportData(result, opts.Exporter.Fields()))
		}
		return explainReport(f, result)
	}
	if opts.List {
		bindings, err := s.Bindings()
		if err != nil {
			return err
		}

		return encoder.Encode(bindings)
	}
	if opts.Identity == "" && !opts.Unset {
		id, err := s.Resolve(opts.Host, opts.Owner, opts.Directory)
		if err != nil {
			return err
		}
		if id != nil {
			cfg, err := f.Config()
			if err != nil {
				return err
			}
			if warning := identity.AccountWarning(cfg.Authentication(), id); warning != "" {
				fmt.Fprintln(f.IOStreams.ErrOut, warning)
			}
		}
		return encoder.Encode(id)
	}
	binding := identity.Binding{
		Identity: opts.Identity, Host: opts.Host, Directory: opts.Directory,
		Owner: opts.Owner,
	}
	executable := f.ExecutablePath
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return err
		}
	}
	if opts.Unset {
		if err := s.Unbind(binding, executable); err != nil {
			return err
		}
		fmt.Fprintln(f.IOStreams.ErrOut, "Removed identity binding")
		return nil
	}
	id, err := s.Load(opts.Identity)
	if err != nil {
		return err
	}
	if err := identity.CheckEnvironment(id.Host); err != nil {
		return err
	}
	cfg, err := f.Config()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(cfg.Authentication().UsersForHost(id.Host), func(user string) bool { return strings.EqualFold(user, id.Account) }) {
		return fmt.Errorf("account %q is no longer saved for %s; run 'gh auth login --hostname %s'", id.Account, id.Host, id.Host)
	}
	if err := s.Bind(binding, executable); err != nil {
		return err
	}
	result, err := s.Explain(id.Host, opts.Owner, opts.Directory)
	if err != nil {
		return fmt.Errorf("identity applied, but could not inspect its scope: %w", err)
	}
	scope := "directory tree at " + result.ScopeDirectory
	if result.CommonGitDir != "" {
		scope = "repository " + result.CommonGitDir + " (main checkout and all worktrees)"
	}
	if opts.Owner != "" {
		scope = "owner " + opts.Owner
	}
	fmt.Fprintf(f.IOStreams.ErrOut, "Using identity %q (%s on %s) for %s\n", id.Name, id.Account, id.Host, scope)
	fmt.Fprintln(f.IOStreams.ErrOut, "Git now uses gh's global include, HTTPS helper and SSH dispatcher. Keep this executable in place until the last choice is removed.")
	fmt.Fprintln(f.IOStreams.ErrOut, "Your saved active account is unchanged.")
	if id.SSHHost != "" {
		fmt.Fprintf(f.IOStreams.ErrOut, "SSH host %q is a connection name; gh has not checked which account its key signs in as.\n", id.SSHHost)
	}
	return nil
}
