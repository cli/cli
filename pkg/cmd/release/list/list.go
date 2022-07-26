package list

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	LimitResults  int
	ExcludeDrafts bool
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

	releases, err := fetchReleases(httpClient, baseRepo, opts.LimitResults, opts.ExcludeDrafts)
	if err != nil {
		return err
	}

	if len(releases) == 0 {
		return cmdutil.NewNoResultsError("no releases found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	now := time.Now()
	table := utils.NewTablePrinter(opts.IO)
	iofmt := opts.IO.ColorScheme()
	for _, rel := range releases {
		title := text.ReplaceExcessiveWhitespace(rel.Name)
		if title == "" {
			title = rel.TagName
		}
		table.AddField(title, nil, nil)

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
		table.AddField(badge, nil, badgeColor)

		tagName := rel.TagName
		if table.IsTTY() {
			tagName = fmt.Sprintf("(%s)", tagName)
		}
		table.AddField(tagName, nil, nil)

		pubDate := rel.PublishedAt
		if rel.PublishedAt.IsZero() {
			pubDate = rel.CreatedAt
		}
		publishedAt := pubDate.Format(time.RFC3339)
		if table.IsTTY() {
			publishedAt = utils.FuzzyAgo(now.Sub(pubDate))
		}
		table.AddField(publishedAt, nil, iofmt.Gray)
		table.EndRow()
	}
	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}
