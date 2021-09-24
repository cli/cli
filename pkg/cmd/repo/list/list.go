package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Exporter   cmdutil.Exporter

	Limit int
	Owner string

	Visibility  string
	Fork        bool
	Source      bool
	Language    string
	Topic       string
	Archived    bool
	NonArchived bool

	Now func() time.Time
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
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
	cmd.Flags().StringVar(&opts.Topic, "topic", "", "Filter by topic")
	cmd.Flags().BoolVar(&opts.Archived, "archived", false, "Show only archived repositories")
	cmd.Flags().BoolVar(&opts.NonArchived, "no-archived", false, "Omit archived repositories")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.RepositoryFields)

	return cmd
}

var defaultFields = []string{"nameWithOwner", "description", "isPrivate", "isFork", "isArchived", "createdAt", "pushedAt"}

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
		Topic:       opts.Topic,
		Archived:    opts.Archived,
		NonArchived: opts.NonArchived,
		Fields:      defaultFields,
	}
	if opts.Exporter != nil {
		filter.Fields = opts.Exporter.Fields()
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	listResult, err := listRepos(httpClient, host, opts.Limit, opts.Owner, filter)
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, listResult.Repositories)
	}

	cs := opts.IO.ColorScheme()
	tp := utils.NewTablePrinter(opts.IO)
	now := opts.Now()

	for _, repo := range listResult.Repositories {
		info := repoInfo(repo)
		infoColor := cs.Gray
		if repo.IsPrivate {
			infoColor = cs.Yellow
		}

		t := repo.PushedAt
		if repo.PushedAt == nil {
			t = &repo.CreatedAt
		}

		tp.AddField(repo.NameWithOwner, nil, cs.Bold)
		tp.AddField(text.ReplaceExcessiveWhitespace(repo.Description), nil, nil)
		tp.AddField(info, nil, infoColor)
		if tp.IsTTY() {
			tp.AddField(utils.FuzzyAgoAbbr(now, *t), nil, cs.Gray)
		} else {
			tp.AddField(t.Format(time.RFC3339), nil, nil)
		}
		tp.EndRow()
	}

	if listResult.FromSearch && opts.Limit > 1000 {
		fmt.Fprintln(opts.IO.ErrOut, "warning: this query uses the Search API which is capped at 1000 results maximum")
	}
	if opts.IO.IsStdoutTTY() {
		hasFilters := filter.Visibility != "" || filter.Fork || filter.Source || filter.Language != "" || filter.Topic != ""
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

func repoInfo(r api.Repository) string {
	var tags []string

	if r.IsPrivate {
		tags = append(tags, "private")
	} else {
		tags = append(tags, "public")
	}
	if r.IsFork {
		tags = append(tags, "fork")
	}
	if r.IsArchived {
		tags = append(tags, "archived")
	}

	return strings.Join(tags, ", ")
}
