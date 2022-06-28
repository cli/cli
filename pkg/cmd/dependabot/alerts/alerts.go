package alerts

import (
	"github.com/cli/cli/v2/pkg/cmd/dependabot/alerts/list"
	"github.com/cli/cli/v2/pkg/cmd/dependabot/alerts/view"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/spf13/cobra"
)

func NewCmdAlerts(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerts <command>",
		Short: "Manage Dependabot Alerts",
		Long:  `View and manage Dependabot Alerts.`,
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(list.NewCmdList(f, nil))
	cmd.AddCommand(view.NewCmdView(f, nil))

	return cmd
}
