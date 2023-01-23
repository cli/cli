package variable

import (
	"github.com/MakeNowJust/heredoc"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/variable/create"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/variable/delete"
	cmdList "github.com/cli/cli/v2/pkg/cmd/variable/list"
	cmdUpdate "github.com/cli/cli/v2/pkg/cmd/variable/update"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdVariable(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variable <command>",
		Short: "Manage GitHub variables",
		Long: heredoc.Doc(`
			Variables can be set at the repository, or organization level for use in
			GitHub Actions. Environment variables can be set for use in
			GitHub Actions. Run "gh help variable create" to learn how to get started.
`),
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdUpdate.NewCmdUpdate(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))

	return cmd
}
