package view

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser
	Exporter   cmdutil.Exporter

	TagName string
	WebMode bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "view [<tag>]",
		Short: "View information about a release",
		Long: heredoc.Doc(`
			View information about a GitHub Release.

			Without an explicit tag name argument, the latest release in the project
			is shown.
		`),
		Args: cobra.MaximumNArgs(1),
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

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the release in the browser")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.ReleaseFields)

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

	ctx := context.Background()
	var release *shared.Release

	if opts.TagName == "" {
		release, err = shared.FetchLatestRelease(ctx, httpClient, baseRepo)
		if err != nil {
			return err
		}
	} else {
		release, err = shared.FetchRelease(ctx, httpClient, baseRepo, opts.TagName)
		if err != nil {
			return err
		}
	}

	if opts.WebMode {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(release.URL))
		}
		return opts.Browser.Browse(release.URL)
	}

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, release)
	}

	if opts.IO.IsStdoutTTY() {
		if err := renderReleaseTTY(opts.IO, release); err != nil {
			return err
		}
	} else {
		if err := renderReleasePlain(opts.IO.Out, release); err != nil {
			return err
		}
	}

	return nil
}

func renderReleaseTTY(io *iostreams.IOStreams, release *shared.Release) error {
	iofmt := io.ColorScheme()
	w := io.Out

	fmt.Fprintf(w, "%s\n", iofmt.Bold(release.TagName))
	if release.IsDraft {
		fmt.Fprintf(w, "%s • ", iofmt.Red("Draft"))
	} else if release.IsPrerelease {
		fmt.Fprintf(w, "%s • ", iofmt.Yellow("Pre-release"))
	}
	if release.IsDraft {
		fmt.Fprintf(w, "%s\n", iofmt.Gray(fmt.Sprintf("%s created this %s", release.Author.Login, text.FuzzyAgo(time.Now(), release.CreatedAt))))
	} else {
		fmt.Fprintf(w, "%s\n", iofmt.Gray(fmt.Sprintf("%s released this %s", release.Author.Login, text.FuzzyAgo(time.Now(), *release.PublishedAt))))
	}

	renderedDescription, err := markdown.Render(release.Body,
		markdown.WithTheme(io.TerminalTheme()),
		markdown.WithWrap(io.TerminalWidth()))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, renderedDescription)

	if len(release.Assets) > 0 {
		fmt.Fprintf(w, "%s\n", iofmt.Bold("Assets"))
		//nolint:staticcheck // SA1019: Showing NAME|SIZE headers adds nothing to table.
		table := tableprinter.New(io, tableprinter.NoHeader)
		for _, a := range release.Assets {
			table.AddField(a.Name)
			table.AddField(humanFileSize(a.Size))
			table.EndRow()
		}
		err := table.Render()
		if err != nil {
			return err
		}
		fmt.Fprint(w, "\n")
	}

	fmt.Fprintf(w, "%s\n", iofmt.Gray(fmt.Sprintf("View on GitHub: %s", release.URL)))
	return nil
}

func renderReleasePlain(w io.Writer, release *shared.Release) error {
	fmt.Fprintf(w, "title:\t%s\n", release.Name)
	fmt.Fprintf(w, "tag:\t%s\n", release.TagName)
	fmt.Fprintf(w, "draft:\t%v\n", release.IsDraft)
	fmt.Fprintf(w, "prerelease:\t%v\n", release.IsPrerelease)
	fmt.Fprintf(w, "author:\t%s\n", release.Author.Login)
	fmt.Fprintf(w, "created:\t%s\n", release.CreatedAt.Format(time.RFC3339))
	if !release.IsDraft {
		fmt.Fprintf(w, "published:\t%s\n", release.PublishedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(w, "url:\t%s\n", release.URL)
	for _, a := range release.Assets {
		fmt.Fprintf(w, "asset:\t%s\n", a.Name)
	}
	fmt.Fprint(w, "--\n")
	fmt.Fprint(w, release.Body)
	if !strings.HasSuffix(release.Body, "\n") {
		fmt.Fprintf(w, "\n")
	}
	return nil
}

func humanFileSize(s int64) string {
	if s < 1024 {
		return fmt.Sprintf("%d B", s)
	}

	kb := float64(s) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%s KiB", floatToString(kb, 2))
	}

	mb := kb / 1024
	if mb < 1024 {
		return fmt.Sprintf("%s MiB", floatToString(mb, 2))
	}

	gb := mb / 1024
	return fmt.Sprintf("%s GiB", floatToString(gb, 2))
}

// render float to fixed precision using truncation instead of rounding
func floatToString(f float64, p uint8) string {
	fs := fmt.Sprintf("%#f%0*s", f, p, "")
	idx := strings.IndexRune(fs, '.')
	return fs[:idx+int(p)+1]
}
