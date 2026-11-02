package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func newDeleteCommand(f *cmdutil.Factory) *cobra.Command {
	return newCleanupCommand(f, false)
}

// NewCmdResetExperiment exposes the existing all-identity cleanup as a return-to-stable command.
func NewCmdResetExperiment(f *cmdutil.Factory) *cobra.Command {
	cmd := newCleanupCommand(f, true)
	cmd.Use = "reset-multi-account-experiment"
	cmd.Short = "Remove experimental identities and bindings before returning to stable gh"
	cmd.Args = cobra.NoArgs
	cmd.Long = heredoc.Doc(`
		Remove all experimental identities and bindings across hosts in this gh configuration,
		including the managed Git include, credential helpers, and SSH dispatcher.

		Run this with the prerelease before replacing or uninstalling it. An older gh cannot
		run the experimental SSH dispatcher left by an active binding.

		Saved accounts, credentials, SSH keys, and your own Git/SSH settings are kept.
		This is the same cleanup as gh auth identity delete --all. It does not uninstall gh.
		Retype "all identities" to confirm, or pass --confirm to skip the prompt.
		Noninteractive use requires --confirm. It is safe to repeat after successful cleanup.
	`)
	cmd.Example = heredoc.Doc(`
		$ gh auth reset-multi-account-experiment
		$ gh auth reset-multi-account-experiment --confirm
	`)
	return cmd
}

func newCleanupCommand(f *cmdutil.Factory, reset bool) *cobra.Command {
	all, confirm := reset, false
	cmd := &cobra.Command{
		Use: "delete [<name>]", Short: "Delete identities and their repository, directory, and owner choices",
		Long: heredoc.Doc(`
			Delete an identity and every repository, directory or owner choice that uses it.
			Deleting a repository choice affects its main checkout and all linked worktrees.
			Use --all to delete all identities and bindings across all hosts in this gh configuration.

			Locations affected by deletion use any remaining applicable identity, or your usual
			gh and Git settings. When no bindings remain, gh removes its managed Git include,
			credential-helper settings, and SSH dispatcher settings.

			Saved accounts, credentials, SSH keys, and your own Git and SSH settings are not deleted.
			You will be asked to retype the identity name, or "all identities" with --all.
			Use --confirm to confirm without prompting; it is required when not running interactively.
		`),
		Example: heredoc.Doc(`
			$ gh auth identity delete work
			$ gh auth identity delete work --confirm
			$ gh auth identity delete --all --confirm
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) > 0 {
				return cmdutil.FlagErrorf("specify an identity name or --all, but not both")
			}
			if !all && len(args) == 0 {
				return cmdutil.FlagErrorf("specify an identity name, or use --all to delete all identities and bindings")
			}
			if !confirm && !f.IOStreams.CanPrompt() {
				return cmdutil.FlagErrorf("--confirm required when not running interactively")
			}
			s, err := identity.Open()
			if err != nil {
				return err
			}
			names := args
			confirmation := "all identities"
			if all {
				ids, err := s.List()
				if err != nil {
					return err
				}
				names = make([]string, 0, len(ids))
				for _, id := range ids {
					names = append(names, id.Name)
				}
			} else {
				if _, err := s.Load(names[0]); err != nil {
					return err
				}
				confirmation = names[0]
			}
			bindings, err := s.Bindings()
			if err != nil {
				return err
			}
			if all {
				fmt.Fprintln(f.IOStreams.ErrOut, "Delete all identities and bindings?")
				for _, name := range names {
					fmt.Fprintf(f.IOStreams.ErrOut, "  Identity %q\n", name)
				}
			} else {
				fmt.Fprintf(f.IOStreams.ErrOut, "Delete identity %q and its bindings?\n", names[0])
			}
			for _, b := range bindings {
				if all || slices.Contains(names, b.Identity) {
					scope := "directory " + b.Directory
					if b.Repository != "" {
						scope = "repository " + filepath.Dir(b.Repository) + " (all worktrees)"
					}
					if b.Owner != "" {
						scope = "owner " + b.Owner
					}
					fmt.Fprintf(f.IOStreams.ErrOut, "  Remove binding for %s on %s\n", scope, b.Host)
				}
			}
			fmt.Fprintln(f.IOStreams.ErrOut, "Repository files, commits, and worktrees will not be deleted.")
			fmt.Fprintln(f.IOStreams.ErrOut, "Affected locations will use remaining identity choices or your usual gh and Git settings.")
			fmt.Fprintln(f.IOStreams.ErrOut, "Saved accounts, credentials, and SSH keys will not be removed.")
			if !confirm {
				fmt.Fprintln(f.IOStreams.ErrOut, "Nothing is deleted until you confirm.")
				if err := f.Prompter.ConfirmDeletion(confirmation); err != nil {
					return err
				}
			}
			executable := f.ExecutablePath
			if executable == "" {
				executable, err = os.Executable()
				if err != nil {
					return err
				}
			}
			if err := s.DeleteIdentities(names, all, executable); err != nil {
				return err
			}
			if all {
				fmt.Fprintln(f.IOStreams.ErrOut, "Deleted all identities and bindings.")
			} else {
				fmt.Fprintf(f.IOStreams.ErrOut, "Deleted identity %q and its bindings.\n", names[0])
			}
			return nil
		},
	}
	if !reset {
		cmd.Flags().BoolVar(&all, "all", false, "Delete all identities and bindings for all hosts")
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm deletion without prompting")
	return cmd
}
