package variable

import (
	"github.com/MakeNowJust/heredoc"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/variable/create"
	cmdList "github.com/cli/cli/v2/pkg/cmd/variable/list"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/variable/delete"
	cmdUpdate "github.com/cli/cli/v2/pkg/cmd/variable/update"
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

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil)) 
	cmd.AddCommand(cmdUpdate.NewCmdUpdate(f, nil)) 
	return cmd
}
