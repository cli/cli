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
			gh run view:    View details for a given workflow run

			%s
			gh job view:   View details for a given job
		`,
		cs.Bold("Working with runs"),
		cs.Bold("Working with jobs within runs")))
	/*
		fmt.Fprint(opts.IO.Out, heredoc.Docf(`
			Welcome to GitHub Actions on the command line.

			%s
			gh workflow list:    List workflows in the current repository
			gh workflow run:     Kick off a workflow run
			gh workflow init:    Create a new workflow
			gh workflow check:   Check a workflow file for correctness

			%s
			gh run list:    List recent workflow runs
			gh run view:    View details for a given workflow run
			gh run watch:   Watch a streaming log for a workflow run

			%s
			gh job view:   View details for a given job
			gh job run:    Run a given job within a workflow
		`,
			cs.Bold("Working with workflows"),
			cs.Bold("Working with runs"),
			cs.Bold("Working with jobs within runs")))
	*/
}
