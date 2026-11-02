package identity

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdIdentity manages experimental reusable account and Git identities.
func NewCmdIdentity(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identity <command>",
		Short: "Manage reusable account and SSH identities (experimental)",
		Long: heredoc.Doc(`
			Save reusable account and Git settings under an identity name.
			An identity references an account you have already signed in to; it does not store tokens.
			Several identities can use the same account with different SSH aliases.
			Creating an identity does not apply it. Use gh auth use NAME to choose where it applies.
			Use these commands to edit generated files; manual edits are unsupported.
			An SSH alias is a connection name. gh does not check which account its key signs in as.
		`),
	}
	cmd.AddCommand(newSaveCommand(f, false), newSaveCommand(f, true))
	cmd.AddCommand(&cobra.Command{
		Use: "list", Short: "List reusable identities as JSON", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := identity.Open()
			if err != nil {
				return err
			}
			ids, err := s.List()
			if err != nil {
				return err
			}

			return writeJSON(f, ids)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "view <name>", Short: "Show an identity as JSON", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := identity.Open()
			if err != nil {
				return err
			}
			id, err := s.Load(args[0])
			if err != nil {
				return err
			}

			return writeJSON(f, id)
		},
	})
	cmd.AddCommand(newDeleteCommand(f))
	return cmd
}

func newSaveCommand(f *cmdutil.Factory, edit bool) *cobra.Command {
	id := identity.Identity{}
	verb := "create"
	if edit {
		verb = "edit"
	}
	cmd := &cobra.Command{
		Use: verb + " <name>", Short: strings.ToUpper(verb[:1]) + verb[1:] + " a reusable identity", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := identity.Open()
			if err != nil {
				return err
			}
			if edit {
				existing, err := s.Load(args[0])
				if err != nil {
					return err
				}
				if !cmd.Flags().Changed("account") {
					id.Account = existing.Account
				}
				id.Host = existing.Host
				if !cmd.Flags().Changed("ssh-host") {
					id.SSHHost = existing.SSHHost
				}

			}

			id.Name = args[0]
			id.Host = strings.ToLower(id.Host)
			cfg, err := f.Config()
			if err != nil {
				return err
			}
			users := cfg.Authentication().UsersForHost(id.Host)
			if !slices.ContainsFunc(users, func(user string) bool { return strings.EqualFold(user, id.Account) }) {
				return fmt.Errorf("account %q is not saved for %s; run 'gh auth login --hostname %s' first", id.Account, id.Host, id.Host)
			}
			for _, user := range users {
				if strings.EqualFold(user, id.Account) {
					id.Account = user
				}
			}
			if err := s.Save(id, edit); err != nil {
				return err
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "Saved identity %q in %s\n", id.Name, s.IdentityPath(id.Name))

			if id.SSHHost != "" {
				fmt.Fprintf(f.IOStreams.ErrOut, "SSH host %q is a connection name; gh has not checked which account its key signs in as.\n", id.SSHHost)
			}
			return nil
		},
	}
	cmd.Long = "Save a reference to an existing saved account. Creating does not apply it. Use gh auth use NAME in a checkout to select its repository family. SSH aliases are connection names, not account verification. Editing preserves omitted values and cannot change the hostname."
	cmd.Example = "$ gh auth identity " + verb + " work --account alice --ssh-host github-work"
	cmd.Flags().StringVar(&id.Account, "account", "", "Saved account `login` to reference (required when creating)")
	if !edit {
		cmd.Flags().StringVar(&id.Host, "hostname", "github.com", "GitHub hostname")
		_ = cmd.MarkFlagRequired("account")
	}
	cmd.Flags().StringVar(&id.SSHHost, "ssh-host", "", "SSH host or alias `name` to use; existence and account are not checked")

	return cmd
}

func writeJSON(f *cmdutil.Factory, value any) error {
	encoder := json.NewEncoder(f.IOStreams.Out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// NewCmdGitSSH is Git's persistent SSH dispatcher for independently invoked commands.
func NewCmdGitSSH(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use: "git-ssh", Hidden: true, DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSSH(f, args)
		},
	}
}
