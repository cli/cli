package release

import (
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/release/create"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/release/delete"
	cmdDeleteAsset "github.com/cli/cli/v2/pkg/cmd/release/delete-asset"
	cmdDownload "github.com/cli/cli/v2/pkg/cmd/release/download"
	cmdUpdate "github.com/cli/cli/v2/pkg/cmd/release/edit"
	cmdList "github.com/cli/cli/v2/pkg/cmd/release/list"
	cmdUpload "github.com/cli/cli/v2/pkg/cmd/release/upload"
	cmdView "github.com/cli/cli/v2/pkg/cmd/release/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdRelease(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "release <command>",
		Short:   "Manage releases",
		GroupID: "core",
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmdutil.AddGroup(cmd, "General commands",
		cmdList.NewCmdList(f, nil),
		cmdCreate.NewCmdCreate(f, nil),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		cmdView.NewCmdView(f, nil),
		cmdUpdate.NewCmdEdit(f, nil),
		cmdUpload.NewCmdUpload(f, nil),
		cmdDownload.NewCmdDownload(f, nil),
		cmdDelete.NewCmdDelete(f, nil),
		cmdDeleteAsset.NewCmdDeleteAsset(f, nil),
	)

	return cmd
}
