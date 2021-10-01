package repo

import (
	"github.com/MakeNowJust/heredoc"
	repoArchiveCmd "github.com/cli/cli/v2/pkg/cmd/repo/archive"
	repoCloneCmd "github.com/cli/cli/v2/pkg/cmd/repo/clone"
	repoCreateCmd "github.com/cli/cli/v2/pkg/cmd/repo/create"
	creditsCmd "github.com/cli/cli/v2/pkg/cmd/repo/credits"
	repoForkCmd "github.com/cli/cli/v2/pkg/cmd/repo/fork"
	gardenCmd "github.com/cli/cli/v2/pkg/cmd/repo/garden"
	repoListCmd "github.com/cli/cli/v2/pkg/cmd/repo/list"
	repoSyncCmd "github.com/cli/cli/v2/pkg/cmd/repo/sync"
	repoViewCmd "github.com/cli/cli/v2/pkg/cmd/repo/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdRepo(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <command>",
		Short: "Create, clone, fork, and view repositories",
		Long:  `Work with GitHub repositories`,
		Example: heredoc.Doc(`
			$ gh repo create
			$ gh repo clone cli/cli
			$ gh repo view --web
		`),
		Annotations: map[string]string{
			"IsCore": "true",
			"help:arguments": heredoc.Doc(`
				A repository can be supplied as an argument in any of the following formats:
				- "OWNER/REPO"
				- by URL, e.g. "https://github.com/OWNER/REPO"
			`),
		},
	}

	cmd.AddCommand(repoViewCmd.NewCmdView(f, nil))
	cmd.AddCommand(repoForkCmd.NewCmdFork(f, nil))
	cmd.AddCommand(repoCloneCmd.NewCmdClone(f, nil))
	cmd.AddCommand(repoCreateCmd.NewCmdCreate(f, nil))
	cmd.AddCommand(repoListCmd.NewCmdList(f, nil))
	cmd.AddCommand(repoSyncCmd.NewCmdSync(f, nil))
	cmd.AddCommand(creditsCmd.NewCmdRepoCredits(f, nil))
	cmd.AddCommand(gardenCmd.NewCmdGarden(f, nil))
	cmd.AddCommand(repoArchiveCmd.NewCmdArchive(f, nil))

	return cmd
}
