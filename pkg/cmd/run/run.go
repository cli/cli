package run

import (
	cmdDownload "github.com/cli/cli/pkg/cmd/run/download"
	cmdList "github.com/cli/cli/pkg/cmd/run/list"
	cmdView "github.com/cli/cli/pkg/cmd/run/view"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdRun(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "run <command>",
		Short:  "View details about workflow runs",
		Hidden: true,
		Long:   "List, view, and watch recent workflow runs from GitHub Actions.",
		// TODO i'd like to have all the actions commands sorted into their own zone which i think will
		// require a new annotation
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))
	cmd.AddCommand(cmdDownload.NewCmdDownload(f, nil))

	return cmd
}
