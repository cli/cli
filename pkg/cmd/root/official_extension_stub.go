package root

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ci"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// NewCmdOfficialExtensionStub creates a hidden stub command for an official
// extension that has not yet been installed. When invoked, it suggests
// installing the extension and, in interactive sessions, offers to do so
// immediately. After a successful install, the extension is dispatched with
// the original arguments.
func NewCmdOfficialExtensionStub(io *iostreams.IOStreams, p prompter.Prompter, em extensions.ExtensionManager, ext *extensions.OfficialExtension) *cobra.Command {
	cmd := &cobra.Command{
		Use:     ext.Name,
		Short:   fmt.Sprintf("Install the official %s extension", ext.Name),
		Hidden:  true,
		GroupID: "extension",
		// Accept any args/flags the user may have passed so we don't get
		// cobra validation errors before reaching RunE.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return officialExtensionStubRun(io, p, em, ext)
		},
	}

	cmdutil.DisableAuthCheck(cmd)

	return cmd
}

func officialExtensionStubRun(io *iostreams.IOStreams, p prompter.Prompter, em extensions.ExtensionManager, ext *extensions.OfficialExtension) error {
	stderr := io.ErrOut

	// In CI, skip the prompt so agents and CI runners don't block on Y/n.
	if !ci.IsCI() {
		if io.CanPrompt() {
			prompt := heredoc.Docf(`
				%[1]s is available as an official extension.
				Would you like to install it now?
			`, fmt.Sprintf("gh %s", ext.Name))
			confirmed, err := p.Confirm(prompt, true)
			if err != nil {
				return err
			}
			if !confirmed {
				return nil
			}
		} else {
			fmt.Fprint(stderr, heredoc.Docf(`
				%[1]s is available as an official extension.
				To install it, run:
				  gh extension install %[2]s/%[3]s
			`, fmt.Sprintf("gh %s", ext.Name), ext.Owner, ext.Repo))
			return cmdutil.SilentError
		}
	}

	repo := ext.Repository()
	io.StartProgressIndicatorWithLabel(fmt.Sprintf("Installing %s/%s...", ext.Owner, ext.Repo))
	installErr := em.Install(repo, "")
	io.StopProgressIndicator()
	if installErr != nil {
		return fmt.Errorf("failed to install extension: %w", installErr)
	}

	fmt.Fprintf(stderr, "Successfully installed %s/%s\n", ext.Owner, ext.Repo)
	return nil
}
