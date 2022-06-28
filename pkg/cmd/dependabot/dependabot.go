package dependabot

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"

	alertsCmd "github.com/cli/cli/v2/pkg/cmd/dependabot/alerts"
	configCmd "github.com/cli/cli/v2/pkg/cmd/dependabot/config"
	secretCmd "github.com/cli/cli/v2/pkg/cmd/dependabot/secret"
)

func NewCmdDependabot(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dependabot <command>",
		Short: "Dependabot",
		Long:  `Configure and interact with GitHub Dependabot.`,
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(alertsCmd.NewCmdAlerts(f))
	cmd.AddCommand(configCmd.NewCmdConfig(f))
	cmd.AddCommand(secretCmd.NewCmdSecret(f))

	return cmd
}
