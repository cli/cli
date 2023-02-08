package variable

import (
	"github.com/MakeNowJust/heredoc"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/variable/create"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdVariable(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variable <command>",
		Short: "Manage GitHub Actions variables",
		Long: heredoc.Doc(`
		Variables can be set at the repository, environment or organization level for use in GitHub Actions.
		Run "gh help variable create" to learn how to get started.
`),
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	return cmd
}
