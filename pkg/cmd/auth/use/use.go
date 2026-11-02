package use

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// UseOptions holds the scope and identity chosen by auth use.
type UseOptions struct {
	Identity  string
	Directory string
	Owner     string
	Host      string
	Unset     bool
	List      bool
	View      bool
}

// NewCmdUse applies or removes an experimental persistent identity binding.
func NewCmdUse(f *cmdutil.Factory, runF func(*UseOptions) error) *cobra.Command {
	opts := &UseOptions{}
	cmd := &cobra.Command{
		Use: "use [identity]", Short: "Use an identity for this repository and its worktrees, or this directory tree (experimental)",
		Long: "Choose an identity for this repository's main checkout and all current and future worktrees. This installs a global Git include, HTTPS helper, and SSH dispatcher. Keep this gh executable in place until the last choice is removed. --view shows the selected identity as JSON; --list shows choices; --unset removes this scope. The saved active account and credentials remain unchanged. Token environment variables block persistent identity use. Unsupported SSH setups, non-gh credential helpers, bare repositories, submodules, and separate Git directories are refused without changing configuration.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Identity = args[0]
			}
			if opts.View && (opts.Identity != "" || opts.Unset || opts.List) {
				return cmdutil.FlagErrorf("--view cannot be combined with an identity, --unset, --list")
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
	cmd.Long += "\nOutside a repository, choose an identity for the directory tree. The main repository location determines inheritance for external worktrees. Repository choices beat the deepest matching directory. --directory targets another location."
	cmd.Long += "\n--owner supplies a fallback behind repository and directory choices. Conflicting owner identities are refused; select a repository explicitly to resolve ambiguity."
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
	fmt.Fprintf(f.IOStreams.ErrOut, "Using identity %q (%s on %s) for this scope\n", id.Name, id.Account, id.Host)
	fmt.Fprintln(f.IOStreams.ErrOut, "Git now uses gh's global include, HTTPS helper and SSH dispatcher. Keep this executable in place until the last choice is removed.")
	fmt.Fprintln(f.IOStreams.ErrOut, "Your saved active account is unchanged.")
	if id.SSHHost != "" {
		fmt.Fprintf(f.IOStreams.ErrOut, "SSH host %q is a connection name; gh has not checked which account its key signs in as.\n", id.SSHHost)
	}
	return nil
}
