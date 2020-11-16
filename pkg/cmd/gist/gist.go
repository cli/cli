package gist

import (
	"github.com/MakeNowJust/heredoc"
	gistCreateCmd "github.com/cli/cli/pkg/cmd/gist/create"
	gistDeleteCmd "github.com/cli/cli/pkg/cmd/gist/delete"
	gistEditCmd "github.com/cli/cli/pkg/cmd/gist/edit"
	gistListCmd "github.com/cli/cli/pkg/cmd/gist/list"
	gistViewCmd "github.com/cli/cli/pkg/cmd/gist/view"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdGist(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gist <command>",
		Short: "Manage gists",
		Long:  `Work with GitHub gists.`,
		Annotations: map[string]string{
			"IsCore": "true",
			"help:arguments": heredoc.Doc(`
				A gist can be supplied as argument in either of the following formats:
				- by ID, e.g. 5b0e0062eb8e9654adad7bb1d81cc75f
				- by URL, e.g. "https://gist.github.com/OWNER/5b0e0062eb8e9654adad7bb1d81cc75f"
			`),
		},
	}

	cmd.AddCommand(gistCreateCmd.NewCmdCreate(f, nil))
	cmd.AddCommand(gistListCmd.NewCmdList(f, nil))
	cmd.AddCommand(gistViewCmd.NewCmdView(f, nil))
	cmd.AddCommand(gistEditCmd.NewCmdEdit(f, nil))
	cmd.AddCommand(gistDeleteCmd.NewCmdDelete(f, nil))

	return cmd
}
