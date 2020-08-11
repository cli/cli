package pr

import (
	"github.com/cli/cli/internal/ghrepo"
	cmdCheckout "github.com/cli/cli/pkg/cmd/pr/checkout"
	cmdClose "github.com/cli/cli/pkg/cmd/pr/close"
	cmdCreate "github.com/cli/cli/pkg/cmd/pr/create"
	cmdDiff "github.com/cli/cli/pkg/cmd/pr/diff"
	cmdList "github.com/cli/cli/pkg/cmd/pr/list"
	cmdMerge "github.com/cli/cli/pkg/cmd/pr/merge"
	cmdReady "github.com/cli/cli/pkg/cmd/pr/ready"
	cmdReopen "github.com/cli/cli/pkg/cmd/pr/reopen"
	cmdReview "github.com/cli/cli/pkg/cmd/pr/review"
	cmdStatus "github.com/cli/cli/pkg/cmd/pr/status"
	cmdView "github.com/cli/cli/pkg/cmd/pr/view"

	"github.com/MakeNowJust/heredoc"
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
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if repo, _ := cmd.Flags().GetString("repo"); repo != "" {
				// NOTE: this mutates the factory
				f.BaseRepo = func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName(repo)
				}
			}
		},
	}

	cmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")

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

	return cmd
}
