package discussion

import (
	"github.com/MakeNowJust/heredoc"
	cmdList "github.com/cli/cli/v2/pkg/cmd/discussion/list"
	cmdView "github.com/cli/cli/v2/pkg/cmd/discussion/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdDiscussion returns the top-level "discussion" command.
func NewCmdDiscussion(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discussion <command>",
		Short: "Work with GitHub Discussions (preview)",
		Long: heredoc.Doc(`
			Working with discussions in the GitHub CLI is in preview and subject to change without notice.
		`),
		Example: heredoc.Doc(`
			$ gh discussion list
			$ gh discussion create --category "General" --title "Hello"
			$ gh discussion view 123
		`),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A discussion can be supplied as argument in any of the following formats:
				- by number, e.g. "123"; or
				- by URL, e.g. "https://github.com/OWNER/REPO/discussions/123".
			`),
		},
		GroupID: "core",
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmdutil.AddGroup(cmd, "General commands",
		cmdList.NewCmdList(f, nil),
	)

	cmdutil.AddGroup(cmd, "Targeted commands",
		cmdView.NewCmdView(f, nil),
	)

	return cmd
}
