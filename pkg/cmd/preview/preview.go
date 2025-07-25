package preview

import (
	"github.com/MakeNowJust/heredoc"
	cmdPrompter "github.com/cli/cli/v2/pkg/cmd/preview/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdPreview(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview <command>",
		Short: "Execute previews for gh features",
		Long: heredoc.Doc(`
			Preview commands are for testing, demonstrative, and development purposes only.
			They should be considered unstable and can change at any time.
		`),
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(cmdPrompter.NewCmdPrompter(f, nil))

	return cmd
}
