package view

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	TagName string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "view <tag>",
		Short: "View information about a release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.TagName = args[0]

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	release, err := fetchRelease(httpClient, baseRepo, opts.TagName)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", release.TagName)

	renderedDescription, err := utils.RenderMarkdown(release.Description)
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.IO.Out, renderedDescription)

	fmt.Fprintf(opts.IO.Out, "%s\n", release.URL)

	return nil
}
