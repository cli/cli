package alias

import (
	"github.com/MakeNowJust/heredoc"
	deleteCmd "github.com/cli/cli/pkg/cmd/alias/delete"
	listCmd "github.com/cli/cli/pkg/cmd/alias/list"
	setCmd "github.com/cli/cli/pkg/cmd/alias/set"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAlias(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias <command>",
		Short: "Create command shortcuts",
		Long: heredoc.Doc(`
			Aliases can be used to make shortcuts for gh commands or to compose multiple commands.

			Run "gh help alias set" to learn more.
		`),
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(deleteCmd.NewCmdDelete(f, nil))
	cmd.AddCommand(listCmd.NewCmdList(f, nil))
	cmd.AddCommand(setCmd.NewCmdSet(f, nil))

	return cmd
}
