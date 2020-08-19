package release

import (
	cmdCreate "github.com/cli/cli/pkg/cmd/release/create"
	cmdList "github.com/cli/cli/pkg/cmd/release/list"
	cmdView "github.com/cli/cli/pkg/cmd/release/view"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdRelease(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <command>",
		Short: "Manage GitHub releases",
		Annotations: map[string]string{
			"IsCore": "true",
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))

	return cmd
}
