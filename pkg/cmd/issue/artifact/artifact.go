// Package artifact holds gh issue artifact, the command group for the
// Markdown documents and links attached to an issue.
package artifact

import (
	"github.com/MakeNowJust/heredoc"
	cmdList "github.com/cli/cli/v2/pkg/cmd/issue/artifact/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdArtifact returns the gh issue artifact command group, which is in
// preview. It inherits -R from gh issue.
func NewCmdArtifact(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact <command>",
		Short: "Work with issue artifacts (preview)",
		Long: heredoc.Docf(`
			Working with issue artifacts in the GitHub CLI is in preview and subject to change without notice.

			Issue artifacts are Markdown documents and links attached to an issue.
			They are not GitHub Actions artifacts; to download those, use %[1]sgh run download%[1]s.
		`, "`"),
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
