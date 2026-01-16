package reviewthread

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdReviewThread(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review-thread <command>",
		Short: "Manage pull request review threads",
		Long: heredoc.Doc(`
			Manage pull request review threads.

			Review threads are created when comments are left on specific lines of code
			during a pull request review. These threads can be resolved or unresolved to
			track which feedback has been addressed.
		`),
		Example: heredoc.Doc(`
			# Resolve a review thread
			$ gh pr review-thread resolve <thread-id>

			# Unresolve a review thread
			$ gh pr review-thread unresolve <thread-id>

			# List unresolved review threads for a PR
			$ gh pr review-thread list 123 --unresolved
		`),
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(NewCmdResolve(f, nil))
	cmd.AddCommand(NewCmdUnresolve(f, nil))
	cmd.AddCommand(NewCmdList(f, nil))

	return cmd
}
