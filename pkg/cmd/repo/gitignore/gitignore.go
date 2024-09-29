package gitignore

import (
	cmdList "github.com/cli/cli/v2/pkg/cmd/repo/gitignore/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdGitIgnore(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gitignore <command>",
		Short: "View available repository .gitignore template",
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
