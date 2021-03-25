package workflow

import (
	cmdDisable "github.com/cli/cli/pkg/cmd/workflow/disable"
	cmdEnable "github.com/cli/cli/pkg/cmd/workflow/enable"
	cmdList "github.com/cli/cli/pkg/cmd/workflow/list"
	cmdRun "github.com/cli/cli/pkg/cmd/workflow/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdWorkflow(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "workflow <command>",
		Short:  "View details about GitHub Actions workflows",
		Hidden: true,
		Long:   "List, view, and run workflows in GitHub Actions.",
		// TODO i'd like to have all the actions commands sorted into their own zone which i think will
		// require a new annotation
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdEnable.NewCmdEnable(f, nil))
	cmd.AddCommand(cmdDisable.NewCmdDisable(f, nil))
	cmd.AddCommand(cmdRun.NewCmdRun(f, nil))

	return cmd
}
