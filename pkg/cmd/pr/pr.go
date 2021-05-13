package pr

import (
	"github.com/MakeNowJust/heredoc"
	cmdCheckout "github.com/cli/cli/pkg/cmd/pr/checkout"
	cmdChecks "github.com/cli/cli/pkg/cmd/pr/checks"
	cmdClose "github.com/cli/cli/pkg/cmd/pr/close"
	cmdComment "github.com/cli/cli/pkg/cmd/pr/comment"
	cmdCreate "github.com/cli/cli/pkg/cmd/pr/create"
	cmdDiff "github.com/cli/cli/pkg/cmd/pr/diff"
	cmdEdit "github.com/cli/cli/pkg/cmd/pr/edit"
	cmdList "github.com/cli/cli/pkg/cmd/pr/list"
	cmdMerge "github.com/cli/cli/pkg/cmd/pr/merge"
	cmdReady "github.com/cli/cli/pkg/cmd/pr/ready"
	cmdReopen "github.com/cli/cli/pkg/cmd/pr/reopen"
	cmdReview "github.com/cli/cli/pkg/cmd/pr/review"
	cmdStatus "github.com/cli/cli/pkg/cmd/pr/status"
	cmdView "github.com/cli/cli/pkg/cmd/pr/view"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Manage pull requests",
		Long:  "Work with GitHub pull requests",
		Example: heredoc.Doc(`
			$ gh pr checkout 353
			$ gh pr create --fill
			$ gh pr view --web
		`),
		Annotations: map[string]string{
			"IsCore": "true",
			"help:arguments": heredoc.Doc(`
				A pull request can be supplied as argument in any of the following formats:
				- by number, e.g. "123";
				- by URL, e.g. "https://github.com/OWNER/REPO/pull/123"; or
				- by the name of its head branch, e.g. "patch-1" or "OWNER:patch-1".
			`),
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdCheckout.NewCmdCheckout(f, nil))
	cmd.AddCommand(cmdClose.NewCmdClose(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdDiff.NewCmdDiff(f, nil))
	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdMerge.NewCmdMerge(f, nil))
	cmd.AddCommand(cmdReady.NewCmdReady(f, nil))
	cmd.AddCommand(cmdReopen.NewCmdReopen(f, nil))
	cmd.AddCommand(cmdReview.NewCmdReview(f, nil))
	cmd.AddCommand(cmdStatus.NewCmdStatus(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))
	cmd.AddCommand(cmdChecks.NewCmdChecks(f, nil))
	cmd.AddCommand(cmdComment.NewCmdComment(f, nil))
	cmd.AddCommand(cmdEdit.NewCmdEdit(f, nil))

	return cmd
}
