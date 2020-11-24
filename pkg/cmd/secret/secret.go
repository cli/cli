package secret

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"

	cmdCreate "github.com/cli/cli/pkg/cmd/secret/create"
	cmdList "github.com/cli/cli/pkg/cmd/secret/list"
)

func NewCmdSecret(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret <command>",
		Short: "Manage GitHub secrets",
		Long: heredoc.Doc(`
			Secrets can be set at the repository or organization level for use in GitHub Actions.
			Run "gh help secret add" to learn how to get started.
`),
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))

	return cmd
}
