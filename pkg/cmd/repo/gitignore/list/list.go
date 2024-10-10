package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HTTPClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List available repository gitignore templates",
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}
	return cmd
}

func listRun(opts *ListOptions) error {
	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	hostname, _ := cfg.Authentication().DefaultHost()
	gitIgnoreTemplates, err := api.RepoGitIgnoreTemplates(client, hostname)
	if err != nil {
		return err
	}

	if len(gitIgnoreTemplates) == 0 {
		return cmdutil.NewNoResultsError("No gitignore templates found")
	}

	return renderGitIgnoreTemplatesTable(gitIgnoreTemplates, opts)
}

func renderGitIgnoreTemplatesTable(gitIgnoreTemplates []string, opts *ListOptions) error {
	t := tableprinter.New(opts.IO, tableprinter.WithHeader("GITIGNORE"))
	for _, gt := range gitIgnoreTemplates {
		t.AddField(gt)
		t.EndRow()
	}

	return t.Render()
}
