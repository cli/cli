package list

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Exporter cmdutil.Exporter

	LimitResults       int
	ExcludeDrafts      bool
	ExcludePreReleases bool
	Order              string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List releases in a repository",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number of items to fetch")
	cmd.Flags().BoolVar(&opts.ExcludeDrafts, "exclude-drafts", false, "Exclude draft releases")
	cmd.Flags().BoolVar(&opts.ExcludePreReleases, "exclude-pre-releases", false, "Exclude pre-releases")
	cmdutil.StringEnumFlag(cmd, &opts.Order, "order", "O", "desc", []string{"asc", "desc"}, "Order of releases returned")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, releaseFields)

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	releases, err := fetchReleases(httpClient, baseRepo, opts.LimitResults, opts.ExcludeDrafts, opts.ExcludePreReleases, opts.Order)
	if err != nil {
		return err
	}

	if len(releases) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError("no releases found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, releases)
	}

	table := tableprinter.New(opts.IO, tableprinter.WithHeader("Title", "Type", "Tag name", "Published"))
	iofmt := opts.IO.ColorScheme()
	for _, rel := range releases {
		title := text.RemoveExcessiveWhitespace(rel.Name)
		if title == "" {
			title = rel.TagName
		}
		table.AddField(title)

		badge := ""
		var badgeColor func(string) string
		if rel.IsLatest {
			badge = "Latest"
			badgeColor = iofmt.Green
		} else if rel.IsDraft {
			badge = "Draft"
			badgeColor = iofmt.Red
		} else if rel.IsPrerelease {
			badge = "Pre-release"
			badgeColor = iofmt.Yellow
		}
		table.AddField(badge, tableprinter.WithColor(badgeColor))

		table.AddField(rel.TagName, tableprinter.WithTruncate(nil))

		pubDate := rel.PublishedAt
		if rel.PublishedAt.IsZero() {
			pubDate = rel.CreatedAt
		}
		table.AddTimeField(time.Now(), pubDate, iofmt.Gray)
		table.EndRow()
	}
	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}
