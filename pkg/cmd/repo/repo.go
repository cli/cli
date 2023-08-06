package repo

import (
	"github.com/MakeNowJust/heredoc"
	repoArchiveCmd "github.com/cli/cli/v2/pkg/cmd/repo/archive"
	repoCloneCmd "github.com/cli/cli/v2/pkg/cmd/repo/clone"
	repoCreateCmd "github.com/cli/cli/v2/pkg/cmd/repo/create"
	creditsCmd "github.com/cli/cli/v2/pkg/cmd/repo/credits"
	repoDeleteCmd "github.com/cli/cli/v2/pkg/cmd/repo/delete"
	deployKeyCmd "github.com/cli/cli/v2/pkg/cmd/repo/deploy-key"
	repoEditCmd "github.com/cli/cli/v2/pkg/cmd/repo/edit"
	repoForkCmd "github.com/cli/cli/v2/pkg/cmd/repo/fork"
	gardenCmd "github.com/cli/cli/v2/pkg/cmd/repo/garden"
	repoListCmd "github.com/cli/cli/v2/pkg/cmd/repo/list"
	repoRenameCmd "github.com/cli/cli/v2/pkg/cmd/repo/rename"
	repoDefaultCmd "github.com/cli/cli/v2/pkg/cmd/repo/setdefault"
	repoSyncCmd "github.com/cli/cli/v2/pkg/cmd/repo/sync"
	repoUnarchiveCmd "github.com/cli/cli/v2/pkg/cmd/repo/unarchive"
	repoViewCmd "github.com/cli/cli/v2/pkg/cmd/repo/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdRepo(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <command>",
		Short: "Manage repositories",
		Long:  `Work with GitHub repositories.`,
		Example: heredoc.Doc(`
			$ gh repo create
			$ gh repo clone cli/cli
			$ gh repo view --web
		`),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A repository can be supplied as an argument in any of the following formats:
				- "OWNER/REPO"
				- by URL, e.g. "https://github.com/OWNER/REPO"
			`),
		},
		GroupID: "core",
	}

	cmdutil.AddGroup(cmd, "General commands",
		repoListCmd.NewCmdList(f, nil),
		repoCreateCmd.NewCmdCreate(f, nil),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		repoViewCmd.NewCmdView(f, nil),
		repoCloneCmd.NewCmdClone(f, nil),
		repoForkCmd.NewCmdFork(f, nil),
		repoDefaultCmd.NewCmdSetDefault(f, nil),
		repoSyncCmd.NewCmdSync(f, nil),
		repoEditCmd.NewCmdEdit(f, nil),
		deployKeyCmd.NewCmdDeployKey(f),
		repoRenameCmd.NewCmdRename(f, nil),
		repoArchiveCmd.NewCmdArchive(f, nil),
		repoUnarchiveCmd.NewCmdUnarchive(f, nil),
		repoDeleteCmd.NewCmdDelete(f, nil),
		creditsCmd.NewCmdRepoCredits(f, nil),
		gardenCmd.NewCmdGarden(f, nil),
	)

	return cmd
}
