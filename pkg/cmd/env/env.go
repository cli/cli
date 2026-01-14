package env

import (
	"github.com/MakeNowJust/heredoc"
	cmdList "github.com/cli/cli/v2/pkg/cmd/env/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type EnvOptions struct{}

func NewCmdEnv(f *cmdutil.Factory, runF func(*EnvOptions) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env <command>",
		Short: "Manage deployment environments",
		Long: heredoc.Doc(`
			Work with GitHub deployment environments.

			To learn more about GitHub deployment environments, see
			<https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-deployments/managing-environments-for-deployment>.
		`),
		Example: heredoc.Doc(`
			$ gh env list
		`),
		GroupID: "core",
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
