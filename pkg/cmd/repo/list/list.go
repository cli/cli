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

type FilterOptions struct {
	Visibility string // private, public
	Fork       bool
	Source     bool
}

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Limit int
	Owner string

	Visibility string
	Fork       bool
	Source     bool

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

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	filter := FilterOptions{
		Visibility: opts.Visibility,
		Fork:       opts.Fork,
		Source:     opts.Source,
	}

	listResult, err := listRepos(httpClient, ghinstance.OverridableDefault(), opts.Limit, opts.Owner, filter)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	tp := utils.NewTablePrinter(opts.IO)
	now := opts.Now()

	for _, repo := range listResult.Repositories {
		info := repo.Info()
		infoColor := cs.Gray
		if repo.IsPrivate {
			infoColor = cs.Yellow
		}

		tp.AddField(repo.NameWithOwner, nil, cs.Bold)
		tp.AddField(text.ReplaceExcessiveWhitespace(repo.Description), nil, nil)
		tp.AddField(info, nil, infoColor)
		if tp.IsTTY() {
			tp.AddField(utils.FuzzyAgoAbbr(now, repo.PushedAt), nil, cs.Gray)
		} else {
			tp.AddField(repo.PushedAt.Format(time.RFC3339), nil, nil)
		}
		tp.EndRow()
	}

	if opts.IO.IsStdoutTTY() {
		hasFilters := filter.Visibility != "" || filter.Fork || filter.Source
		title := listHeader(listResult.Owner, len(listResult.Repositories), listResult.TotalCount, hasFilters)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	return tp.Render()
}

func listHeader(owner string, matchCount, totalMatchCount int, hasFilters bool) string {
	if totalMatchCount == 0 {
		if hasFilters {
			return "No results match your search"
		}
		return "There are no repositories in @" + owner
	}

	return fmt.Sprintf("Showing %d of %d repositories in @%s", matchCount, totalMatchCount, owner)
}
