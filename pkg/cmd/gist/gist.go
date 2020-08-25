package gist

import (
	gistCreateCmd "github.com/cli/cli/pkg/cmd/gist/create"
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

	return cmd
}
