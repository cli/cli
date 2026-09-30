// Package artifact holds gh issue artifact, the command group for the
// Markdown documents and links attached to an issue.
package artifact

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/issue/artifact/create"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/issue/artifact/delete"
	cmdDownload "github.com/cli/cli/v2/pkg/cmd/issue/artifact/download"
	cmdEdit "github.com/cli/cli/v2/pkg/cmd/issue/artifact/edit"
	cmdList "github.com/cli/cli/v2/pkg/cmd/issue/artifact/list"
	cmdView "github.com/cli/cli/v2/pkg/cmd/issue/artifact/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdArtifact returns the gh issue artifact command group, which is in
// preview. It inherits -R from gh issue. telemetry records --attach use, as
// in the other commands that take it.
func NewCmdArtifact(f *cmdutil.Factory, telemetry ghtelemetry.InvocationRecorder) *cobra.Command {
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
	cmd.AddCommand(cmdView.NewCmdView(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, telemetry, nil))
	cmd.AddCommand(cmdEdit.NewCmdEdit(f, telemetry, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))
	cmd.AddCommand(cmdDownload.NewCmdDownload(f, nil))

	return cmd
}
