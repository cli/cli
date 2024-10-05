package commits

import (
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/search/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

type CommitsOptions struct {
	Browser  browser.Browser
	Exporter cmdutil.Exporter
	IO       *iostreams.IOStreams
	Now      time.Time
	Query    search.Query
	Searcher search.Searcher
	WebMode  bool
}

func NewCmdCommits(f *cmdutil.Factory, runF func(*CommitsOptions) error) *cobra.Command {
	var order string
	var sort string
	opts := &CommitsOptions{
		Browser: f.Browser,
		IO:      f.IOStreams,
		Query:   search.Query{Kind: search.KindCommits},
	}

	cmd := &cobra.Command{
		Use:   "commits [<query>]",
		Short: "Search for commits",
		Long: heredoc.Doc(`
			Search for commits on GitHub.

			The command supports constructing queries using the GitHub search syntax,
			using the parameter and qualifier flags, or a combination of the two.

			GitHub search syntax is documented at:
			<https://docs.github.com/search-github/searching-on-github/searching-commits>
		`),
		Example: heredoc.Doc(`
			# search commits matching set of keywords "readme" and "typo"
			$ gh search commits readme typo

			# search commits matching phrase "bug fix"
			$ gh search commits "bug fix"

			# search commits committed by user "monalisa"
			$ gh search commits --committer=monalisa

			# search commits authored by users with name "Jane Doe"
			$ gh search commits --author-name="Jane Doe"

			# search commits matching hash "8dd03144ffdc6c0d486d6b705f9c7fba871ee7c3"
			$ gh search commits --hash=8dd03144ffdc6c0d486d6b705f9c7fba871ee7c3

			# search commits authored before February 1st, 2022
			$ gh search commits --author-date="<2022-02-01"
		`),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 && c.Flags().NFlag() == 0 {
				return cmdutil.FlagErrorf("specify search keywords or flags")
			}
			if opts.Query.Limit < 1 || opts.Query.Limit > shared.SearchMaxResults {
				return cmdutil.FlagErrorf("`--limit` must be between 1 and 1000")
			}
			if c.Flags().Changed("order") {
				opts.Query.Order = order
			}
			if c.Flags().Changed("sort") {
				opts.Query.Sort = sort
			}
			opts.Query.Keywords = args
			if runF != nil {
				return runF(opts)
			}
			var err error
			opts.Searcher, err = shared.Searcher(f)
			if err != nil {
				return err
			}
			return commitsRun(opts)
		},
	}

	// Output flags
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, search.CommitFields)
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the search query in the web browser")

	// Query parameter flags
	cmd.Flags().IntVarP(&opts.Query.Limit, "limit", "L", 30, "Maximum number of commits to fetch")
	cmdutil.StringEnumFlag(cmd, &order, "order", "", "desc", []string{"asc", "desc"}, "Order of commits returned, ignored unless '--sort' flag is specified")
	cmdutil.StringEnumFlag(cmd, &sort, "sort", "", "best-match", []string{"author-date", "committer-date"}, "Sort fetched commits")

	// Query qualifier flags
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Author, "author", "", "Filter by author")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.AuthorDate, "author-date", "", "Filter based on authored `date`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.AuthorEmail, "author-email", "", "Filter on author email")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.AuthorName, "author-name", "", "Filter on author name")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Committer, "committer", "", "Filter by committer")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.CommitterDate, "committer-date", "", "Filter based on committed `date`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.CommitterEmail, "committer-email", "", "Filter on committer email")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.CommitterName, "committer-name", "", "Filter on committer name")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Hash, "hash", "", "Filter by commit hash")
	cmdutil.NilBoolFlag(cmd, &opts.Query.Qualifiers.Merge, "merge", "", "Filter on merge commits")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Parent, "parent", "", "Filter by parent hash")
	cmd.Flags().StringSliceVarP(&opts.Query.Qualifiers.Repo, "repo", "R", nil, "Filter on repository")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Tree, "tree", "", "Filter by tree hash")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.User, "owner", nil, "Filter on repository owner")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Query.Qualifiers.Is, "visibility", "", nil, []string{"public", "private", "internal"}, "Filter based on repository visibility")

	return cmd
}

func commitsRun(opts *CommitsOptions) error {
	io := opts.IO
	if opts.WebMode {
		url := opts.Searcher.URL(opts.Query)
		if io.IsStdoutTTY() {
			fmt.Fprintf(io.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
	}
	io.StartProgressIndicator()
	result, err := opts.Searcher.Commits(opts.Query)
	io.StopProgressIndicator()
	if err != nil {
		return err
	}
	if len(result.Items) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError("no commits matched your search")
	}
	if err := io.StartPager(); err == nil {
		defer io.StopPager()
	} else {
		fmt.Fprintf(io.ErrOut, "failed to start pager: %v\n", err)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(io, result.Items)
	}

	return displayResults(io, opts.Now, result)
}

func displayResults(io *iostreams.IOStreams, now time.Time, results search.CommitsResult) error {
	if now.IsZero() {
		now = time.Now()
	}
	cs := io.ColorScheme()
	tp := tableprinter.New(io, tableprinter.WithHeader("Repo", "SHA", "Message", "Author", "Created"))
	for _, commit := range results.Items {
		tp.AddField(commit.Repo.FullName)
		tp.AddField(commit.Sha)
		tp.AddField(text.RemoveExcessiveWhitespace(commit.Info.Message))
		tp.AddField(commit.Author.Login)
		tp.AddTimeField(now, commit.Info.Author.Date, cs.Gray)
		tp.EndRow()
	}
	if io.IsStdoutTTY() {
		header := fmt.Sprintf("Showing %d of %d commits\n\n", len(results.Items), results.Total)
		fmt.Fprintf(io.Out, "\n%s", header)
	}
	return tp.Render()
}
