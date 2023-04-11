package repos

import (
	"fmt"
	"strings"
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

type ReposOptions struct {
	Browser  browser.Browser
	Exporter cmdutil.Exporter
	IO       *iostreams.IOStreams
	Now      time.Time
	Query    search.Query
	Searcher search.Searcher
	WebMode  bool
}

func NewCmdRepos(f *cmdutil.Factory, runF func(*ReposOptions) error) *cobra.Command {
	var order string
	var sort string
	opts := &ReposOptions{
		Browser: f.Browser,
		IO:      f.IOStreams,
		Query:   search.Query{Kind: search.KindRepositories},
	}

	cmd := &cobra.Command{
		Use:   "repos [<query>]",
		Short: "Search for repositories",
		Long: heredoc.Doc(`
			Search for repositories on GitHub.

			The command supports constructing queries using the GitHub search syntax,
			using the parameter and qualifier flags, or a combination of the two.

			GitHub search syntax is documented at:
			<https://docs.github.com/search-github/searching-on-github/searching-for-repositories>
    `),
		Example: heredoc.Doc(`
			# search repositories matching set of keywords "cli" and "shell"
			$ gh search repos cli shell

			# search repositories matching phrase "vim plugin"
			$ gh search repos "vim plugin"

			# search repositories public repos in the microsoft organization
			$ gh search repos --owner=microsoft --visibility=public

			# search repositories with a set of topics
			$ gh search repos --topic=unix,terminal

			# search repositories by coding language and number of good first issues
			$ gh search repos --language=go --good-first-issues=">=10"

			# search repositories without topic "linux"
			$ gh search repos -- -topic:linux
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
			return reposRun(opts)
		},
	}

	// Output flags
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, search.RepositoryFields)
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the search query in the web browser")

	// Query parameter flags
	cmd.Flags().IntVarP(&opts.Query.Limit, "limit", "L", 30, "Maximum number of repositories to fetch")
	cmdutil.StringEnumFlag(cmd, &order, "order", "", "desc", []string{"asc", "desc"}, "Order of repositories returned, ignored unless '--sort' flag is specified")
	cmdutil.StringEnumFlag(cmd, &sort, "sort", "", "best-match", []string{"forks", "help-wanted-issues", "stars", "updated"}, "Sort fetched repositories")

	// Query qualifier flags
	cmdutil.NilBoolFlag(cmd, &opts.Query.Qualifiers.Archived, "archived", "", "Filter based on archive state")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Created, "created", "", "Filter based on created at `date`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Followers, "followers", "", "Filter based on `number` of followers")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Qualifiers.Fork, "include-forks", "", "", []string{"false", "true", "only"}, "Include forks in fetched repositories")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Forks, "forks", "", "Filter on `number` of forks")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.GoodFirstIssues, "good-first-issues", "", "Filter on `number` of issues with the 'good first issue' label")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.HelpWantedIssues, "help-wanted-issues", "", "Filter on `number` of issues with the 'help wanted' label")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Query.Qualifiers.In, "match", "", nil, []string{"name", "description", "readme"}, "Restrict search to specific field of repository")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Query.Qualifiers.Is, "visibility", "", nil, []string{"public", "private", "internal"}, "Filter based on visibility")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Language, "language", "", "Filter based on the coding language")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.License, "license", nil, "Filter based on license type")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Pushed, "updated", "", "Filter on last updated at `date`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Size, "size", "", "Filter on a size range, in kilobytes")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Stars, "stars", "", "Filter on `number` of stars")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.Topic, "topic", nil, "Filter on topic")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Topics, "number-topics", "", "Filter on `number` of topics")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.User, "owner", nil, "Filter on owner")

	return cmd
}

func reposRun(opts *ReposOptions) error {
	io := opts.IO
	if opts.WebMode {
		url := opts.Searcher.URL(opts.Query)
		if io.IsStdoutTTY() {
			fmt.Fprintf(io.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
	}
	io.StartProgressIndicator()
	result, err := opts.Searcher.Repositories(opts.Query)
	io.StopProgressIndicator()
	if err != nil {
		return err
	}
	if len(result.Items) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError("no repositories matched your search")
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

func displayResults(io *iostreams.IOStreams, now time.Time, results search.RepositoriesResult) error {
	if now.IsZero() {
		now = time.Now()
	}
	cs := io.ColorScheme()
	tp := tableprinter.New(io)
	tp.HeaderRow("Name", "Description", "Visibility", "Updated")
	for _, repo := range results.Items {
		tags := []string{visibilityLabel(repo)}
		if repo.IsFork {
			tags = append(tags, "fork")
		}
		if repo.IsArchived {
			tags = append(tags, "archived")
		}
		info := strings.Join(tags, ", ")
		infoColor := cs.Gray
		if repo.IsPrivate {
			infoColor = cs.Yellow
		}
		tp.AddField(repo.FullName, tableprinter.WithColor(cs.Bold))
		tp.AddField(text.RemoveExcessiveWhitespace(repo.Description))
		tp.AddField(info, tableprinter.WithColor(infoColor))
		tp.AddTimeField(now, repo.UpdatedAt, cs.Gray)
		tp.EndRow()
	}
	if io.IsStdoutTTY() {
		header := fmt.Sprintf("Showing %d of %d repositories\n\n", len(results.Items), results.Total)
		fmt.Fprintf(io.Out, "\n%s", header)
	}
	return tp.Render()
}

func visibilityLabel(repo search.Repository) string {
	if repo.Visibility != "" {
		return repo.Visibility
	} else if repo.IsPrivate {
		return "private"
	}
	return "public"
}
