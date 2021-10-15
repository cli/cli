package release

import (
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/release/create"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/release/delete"
	cmdDownload "github.com/cli/cli/v2/pkg/cmd/release/download"
	cmdList "github.com/cli/cli/v2/pkg/cmd/release/list"
	cmdUpload "github.com/cli/cli/v2/pkg/cmd/release/upload"
	cmdView "github.com/cli/cli/v2/pkg/cmd/release/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
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
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))
	cmd.AddCommand(cmdDownload.NewCmdDownload(f, nil))
	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))
	cmd.AddCommand(cmdUpload.NewCmdUpload(f, nil))

	return cmd
}
