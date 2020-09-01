package view

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/release/shared"
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
		Use:   "view [<tag>]",
		Short: "View information about a release",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.TagName = args[0]
			}

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

	var release *shared.Release

	if opts.TagName == "" {
		release, err = shared.FetchLatestRelease(httpClient, baseRepo)
		if err != nil {
			return err
		}
	} else {
		release, err = shared.FetchRelease(httpClient, baseRepo, opts.TagName)
		if err != nil {
			return err
		}
	}

	iofmt := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Out, "%s\n", iofmt.Bold(release.TagName))
	if release.IsDraft {
		fmt.Fprintf(opts.IO.Out, "%s • ", iofmt.Red("Draft"))
	} else if release.IsPrerelease {
		fmt.Fprintf(opts.IO.Out, "%s • ", iofmt.Yellow("Pre-release"))
	}
	if release.IsDraft {
		fmt.Fprintf(opts.IO.Out, "%s\n", iofmt.Gray(fmt.Sprintf("%s created this %s", release.Author.Login, utils.FuzzyAgo(time.Since(release.CreatedAt)))))
	} else {
		fmt.Fprintf(opts.IO.Out, "%s\n", iofmt.Gray(fmt.Sprintf("%s released this %s", release.Author.Login, utils.FuzzyAgo(time.Since(release.PublishedAt)))))
	}

	renderedDescription, err := utils.RenderMarkdown(release.Body)
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.IO.Out, renderedDescription)

	if len(release.Assets) > 0 {
		fmt.Fprintf(opts.IO.Out, "%s\n", iofmt.Bold("Assets"))
		table := utils.NewTablePrinter(opts.IO)
		for _, a := range release.Assets {
			table.AddField(a.Name, nil, nil)
			table.AddField(humanFileSize(a.Size), nil, nil)
			table.EndRow()
		}
		err := table.Render()
		if err != nil {
			return err
		}
		fmt.Fprint(opts.IO.Out, "\n")
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", iofmt.Gray(fmt.Sprintf("View on GitHub: %s", release.HTMLURL)))

	return nil
}

func humanFileSize(s int64) string {
	if s < 1024 {
		return fmt.Sprintf("%d B", s)
	}

	kb := float64(s) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.2f KiB", kb)
	}

	mb := float64(kb) / 1024
	if mb < 1024 {
		return fmt.Sprintf("%.2f MiB", mb)
	}

	gb := float64(kb) / 1024
	return fmt.Sprintf("%.2f GiB", gb)
}
