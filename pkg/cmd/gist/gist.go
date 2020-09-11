package gist

import (
	gistCreateCmd "github.com/cli/cli/pkg/cmd/gist/create"
	gistEditCmd "github.com/cli/cli/pkg/cmd/gist/edit"
	gistListCmd "github.com/cli/cli/pkg/cmd/gist/list"
	gistViewCmd "github.com/cli/cli/pkg/cmd/gist/view"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdGist(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gist",
		Short: "Create gists",
		Long:  `Work with GitHub gists.`,
	}

	cmd.AddCommand(gistCreateCmd.NewCmdCreate(f, nil))
	cmd.AddCommand(gistListCmd.NewCmdList(f, nil))
	cmd.AddCommand(gistViewCmd.NewCmdView(f, nil))
	cmd.AddCommand(gistEditCmd.NewCmdEdit(f, nil))

	return cmd
}
