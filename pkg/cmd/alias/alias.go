package alias

import (
	"github.com/MakeNowJust/heredoc"
	deleteCmd "github.com/cli/cli/v2/pkg/cmd/alias/delete"
	importCmd "github.com/cli/cli/v2/pkg/cmd/alias/imports"
	listCmd "github.com/cli/cli/v2/pkg/cmd/alias/list"
	setCmd "github.com/cli/cli/v2/pkg/cmd/alias/set"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAlias(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias <command>",
		Short: "Create command shortcuts",
		Long: heredoc.Docf(`
			Aliases can be used to make shortcuts for gh commands or to compose multiple commands.

			Run %[1]sgh help alias set%[1]s to learn more.
		`, "`"),
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(deleteCmd.NewCmdDelete(f, nil))
	cmd.AddCommand(importCmd.NewCmdImport(f, nil))
	cmd.AddCommand(listCmd.NewCmdList(f, nil))
	cmd.AddCommand(setCmd.NewCmdSet(f, nil))

	return cmd
}
