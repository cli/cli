package actions

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ActionsOptions struct {
	IO *iostreams.IOStreams
}

func NewCmdActions(f *cmdutil.Factory) *cobra.Command {
	opts := ActionsOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:    "actions",
		Short:  "Learn about working with GitHub actions",
		Args:   cobra.ExactArgs(0),
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			actionsRun(opts)
		},
		Annotations: map[string]string{
			"IsActions": "true",
		},
	}

	return cmd
}

func actionsRun(opts ActionsOptions) {
	cs := opts.IO.ColorScheme()
	fmt.Fprint(opts.IO.Out, heredoc.Docf(`
			Welcome to GitHub Actions on the command line.

			This part of gh is in beta and subject to change!

			To follow along while we get to GA, please see this
			tracking issue: https://github.com/cli/cli/issues/2889

			%s
			gh run list:    List recent workflow runs
			gh run view:    View details for a workflow run or one of its jobs
			gh run watch:   Watch a workflow run while it executes
			gh run rerun:   Rerun a failed workflow run

			%s
			gh workflow list:      List all the workflow files in your repository
			gh workflow enable:    Enable a workflow file
			gh workflow disable:   Disable a workflow file
			gh workflow run:       Trigger a workflow_dispatch run for a workflow file
		`,
		cs.Bold("Interacting with workflow runs"),
		cs.Bold("Interacting with workflow files")))
}
