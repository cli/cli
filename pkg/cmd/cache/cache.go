package cache

import (
	"github.com/MakeNowJust/heredoc"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/cache/delete"
	cmdList "github.com/cli/cli/v2/pkg/cmd/cache/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdCache(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache <command>",
		Short: "Manage GitHub Actions caches",
		Long:  "Work with GitHub Actions caches.",
		Example: heredoc.Doc(`
			$ gh cache list
			$ gh cache delete --all
		`),
		GroupID: "actions",
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))

	return cmd
}
