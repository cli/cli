package job

import (
	viewCmd "github.com/cli/cli/pkg/cmd/job/view"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdJob(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job <command>",
		Short: "Interact with the individual jobs of a workflow run",
		Long:  "List and view the jobs of a workflow run including full logs",
		// TODO action annotation
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(viewCmd.NewCmdView(f, nil))

	return cmd
}
