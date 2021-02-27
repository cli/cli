package list

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Limit int
	Owner string

	Visibility  string
	Fork        bool
	Source      bool
	Language    string
	Archived    bool
	NonArchived bool

	Now func() time.Time
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}

	var (
		flagPublic  bool
		flagPrivate bool
	)

	cmd := &cobra.Command{
		Use:   "list [<owner>]",
		Args:  cobra.MaximumNArgs(1),
		Short: "List repositories owned by user or organization",
		RunE: func(c *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.Limit)}
			}

			if flagPrivate && flagPublic {
				return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of `--public` or `--private`")}
			}
			if opts.Source && opts.Fork {
				return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of `--source` or `--fork`")}
			}
			if opts.Archived && opts.NonArchived {
				return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of `--archived` or `--no-archived`")}
			}

			if flagPrivate {
				opts.Visibility = "private"
			} else if flagPublic {
				opts.Visibility = "public"
			}

			if len(args) > 0 {
				opts.Owner = args[0]
			}

			if runF != nil {
				return runF(&opts)
			}
			return listRun(&opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of repositories to list")
	cmd.Flags().BoolVar(&flagPrivate, "private", false, "Show only private repositories")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Show only public repositories")
	cmd.Flags().BoolVar(&opts.Source, "source", false, "Show only non-forks")
	cmd.Flags().BoolVar(&opts.Fork, "fork", false, "Show only forks")
	cmd.Flags().StringVarP(&opts.Language, "language", "l", "", "Filter by primary coding language")
	cmd.Flags().BoolVar(&opts.Archived, "archived", false, "Show only archived repositories")
	cmd.Flags().BoolVar(&opts.NonArchived, "no-archived", false, "Omit archived repositories")

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	filter := FilterOptions{
		Visibility:  opts.Visibility,
		Fork:        opts.Fork,
		Source:      opts.Source,
		Language:    opts.Language,
		Archived:    opts.Archived,
		NonArchived: opts.NonArchived,
	}

	listResult, err := listRepos(httpClient, ghinstance.OverridableDefault(), opts.Limit, opts.Owner, filter)
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	cs := opts.IO.ColorScheme()
	tp := utils.NewTablePrinter(opts.IO)
	now := opts.Now()

	for _, repo := range listResult.Repositories {
		info := repo.Info()
		infoColor := cs.Gray
		if repo.IsPrivate {
			infoColor = cs.Yellow
		}

		t := repo.PushedAt
		// if listResult.FromSearch {
		// 	t = repo.UpdatedAt
		// }

		tp.AddField(repo.NameWithOwner, nil, cs.Bold)
		tp.AddField(text.ReplaceExcessiveWhitespace(repo.Description), nil, nil)
		tp.AddField(info, nil, infoColor)
		if tp.IsTTY() {
			tp.AddField(utils.FuzzyAgoAbbr(now, t), nil, cs.Gray)
		} else {
			tp.AddField(t.Format(time.RFC3339), nil, nil)
		}
		tp.EndRow()
	}

	if opts.IO.IsStdoutTTY() {
		hasFilters := filter.Visibility != "" || filter.Fork || filter.Source || filter.Language != ""
		title := listHeader(listResult.Owner, len(listResult.Repositories), listResult.TotalCount, hasFilters)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	return tp.Render()
}

func listHeader(owner string, matchCount, totalMatchCount int, hasFilters bool) string {
	if totalMatchCount == 0 {
		if hasFilters {
			return "No results match your search"
		} else if owner != "" {
			return "There are no repositories in @" + owner
		}
		return "No results"
	}

	var matchStr string
	if hasFilters {
		matchStr = " that match your search"
	}
	return fmt.Sprintf("Showing %d of %d repositories in @%s%s", matchCount, totalMatchCount, owner, matchStr)
}
