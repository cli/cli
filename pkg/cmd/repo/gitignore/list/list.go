package list

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/repo/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
	Config     func() (gh.Config, error)
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		GitHubREST: f.GitHubREST,
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
			return listRun(cmd.Context(), opts)
		},
	}
	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	hostname, _ := cfg.Authentication().DefaultHost()

	client, err := opts.GitHubREST(hostname)
	if err != nil {
		return err
	}

	gitIgnoreTemplates, err := shared.RepoGitIgnoreTemplates(ctx, client, hostname)
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
