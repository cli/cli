package secret

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"

	cmdList "github.com/cli/cli/pkg/cmd/secret/list"
	cmdRemove "github.com/cli/cli/pkg/cmd/secret/remove"
	cmdSet "github.com/cli/cli/pkg/cmd/secret/set"
)

func NewCmdSecret(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret <command>",
		Short: "Manage GitHub secrets",
		Long: heredoc.Doc(`
			Secrets can be set at the repository or organization level for use in GitHub Actions.
			Run "gh help secret set" to learn how to get started.
`),
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdSet.NewCmdSet(f, nil))
	cmd.AddCommand(cmdRemove.NewCmdRemove(f, nil))

	return cmd
}
