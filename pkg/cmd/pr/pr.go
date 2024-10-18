package pr

import (
	"github.com/MakeNowJust/heredoc"
	cmdLock "github.com/cli/cli/v2/pkg/cmd/issue/lock"
	cmdCheckout "github.com/cli/cli/v2/pkg/cmd/pr/checkout"
	cmdChecks "github.com/cli/cli/v2/pkg/cmd/pr/checks"
	cmdCleanup "github.com/cli/cli/v2/pkg/cmd/pr/cleanup"
	cmdClose "github.com/cli/cli/v2/pkg/cmd/pr/close"
	cmdComment "github.com/cli/cli/v2/pkg/cmd/pr/comment"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/pr/create"
	cmdDiff "github.com/cli/cli/v2/pkg/cmd/pr/diff"
	cmdEdit "github.com/cli/cli/v2/pkg/cmd/pr/edit"
	cmdList "github.com/cli/cli/v2/pkg/cmd/pr/list"
	cmdMerge "github.com/cli/cli/v2/pkg/cmd/pr/merge"
	cmdReady "github.com/cli/cli/v2/pkg/cmd/pr/ready"
	cmdReopen "github.com/cli/cli/v2/pkg/cmd/pr/reopen"
	cmdReview "github.com/cli/cli/v2/pkg/cmd/pr/review"
	cmdStatus "github.com/cli/cli/v2/pkg/cmd/pr/status"
	cmdUpdateBranch "github.com/cli/cli/v2/pkg/cmd/pr/update-branch"
	cmdView "github.com/cli/cli/v2/pkg/cmd/pr/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Manage pull requests",
		Long:  "Work with GitHub pull requests.",
		Example: heredoc.Doc(`
			$ gh pr checkout 353
			$ gh pr create --fill
			$ gh pr view --web
		`),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A pull request can be supplied as argument in any of the following formats:
				- by number, e.g. "123";
				- by URL, e.g. "https://github.com/OWNER/REPO/pull/123"; or
				- by the name of its head branch, e.g. "patch-1" or "OWNER:patch-1".
			`),
		},
		GroupID: "core",
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmdutil.AddGroup(cmd, "General commands",
		cmdList.NewCmdList(f, nil),
		cmdCreate.NewCmdCreate(f, nil),
		cmdStatus.NewCmdStatus(f, nil),
		cmdCleanup.NewCmdCleanup(f, nil),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		cmdView.NewCmdView(f, nil),
		cmdDiff.NewCmdDiff(f, nil),
		cmdCheckout.NewCmdCheckout(f, nil),
		cmdChecks.NewCmdChecks(f, nil),
		cmdReview.NewCmdReview(f, nil),
		cmdMerge.NewCmdMerge(f, nil),
		cmdUpdateBranch.NewCmdUpdateBranch(f, nil),
		cmdReady.NewCmdReady(f, nil),
		cmdComment.NewCmdComment(f, nil),
		cmdClose.NewCmdClose(f, nil),
		cmdReopen.NewCmdReopen(f, nil),
		cmdEdit.NewCmdEdit(f, nil),
		cmdLock.NewCmdLock(f, cmd.Name(), nil),
		cmdLock.NewCmdUnlock(f, cmd.Name(), nil),
	)

	return cmd
}
