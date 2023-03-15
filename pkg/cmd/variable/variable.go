package variable

import (
	"github.com/MakeNowJust/heredoc"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/variable/delete"
	cmdList "github.com/cli/cli/v2/pkg/cmd/variable/list"
	cmdSet "github.com/cli/cli/v2/pkg/cmd/variable/set"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdVariable(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variable <command>",
		Short: "Manage GitHub Actions variables",
		Long: heredoc.Doc(`
			Variables can be set at the repository, environment or organization level for use in
			GitHub Actions or Dependabot. Run "gh help variable set" to learn how to get started.
    `),
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdSet.NewCmdSet(f, nil))
	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))

	return cmd
}
