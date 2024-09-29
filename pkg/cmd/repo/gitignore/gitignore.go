package gitignore

import (
	cmdList "github.com/cli/cli/v2/pkg/cmd/repo/gitignore/list"
	cmdView "github.com/cli/cli/v2/pkg/cmd/repo/gitignore/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdGitIgnore(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gitignore <command>",
		Short: "List and view available repository gitignore templates",
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))

	return cmd
}
