package list

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmd/discussion/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	defaultLimit = 30

	stateOpen   = "open"
	stateClosed = "closed"
	stateAll    = "all"

	sortCreated = "created"
	sortUpdated = "updated"

	orderAsc  = "asc"
	orderDesc = "desc"
)

// discussionListFields lists the field names available for --json output
// on the discussion list command. This excludes fields like "comments"
// that are only populated by the view command.
var discussionListFields = []string{
	"id",
	"number",
	"title",
	"body",
	"url",
	"closed",
	"stateReason",
	"author",
	"category",
	"labels",
	"answered",
	"answerChosenAt",
	"answerChosenBy",
	"createdAt",
	"updatedAt",
	"closedAt",
	"locked",
}

// ListOptions holds the configuration for the discussion list command.
type ListOptions struct {
	IO       *iostreams.IOStreams
	BaseRepo func() (ghrepo.Interface, error)
	Browser  browser.Browser
	Client   func() (client.DiscussionClient, error)

	Author   string
	Category string
	Labels   []string
	State    string
	Limit    int
	Answered *bool
	Sort     string
	Order    string
	Search   string
	After    string

	WebMode  bool
	Exporter cmdutil.Exporter
	Now      func() time.Time
}

// NewCmdList creates the "discussion list" command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:      f.IOStreams,
		Browser: f.Browser,
		Now:     time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list [flags]",
		Short: "List discussions in a repository (preview)",
		Long: heredoc.Doc(`
			List discussions in a GitHub repository. By default, only open discussions
			are shown.
		`),
		Example: heredoc.Doc(`
			# List open discussions
			$ gh discussion list

			# List discussions with a specific category
			$ gh discussion list --category General

			# List closed discussions by author
			$ gh discussion list --state closed --author monalisa

			# List all discussions (closed or open) by label
			$ gh discussion list --state all --label bug,enhancement

			# List answered Q&A discussions as JSON
			$ gh discussion list --answered --json number,title,url

			# List unanswered Q&A discussions as JSON
			$ gh discussion list --answered=false --json number,title,url
		`),
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.Client = shared.DiscussionClientFunc(f)

			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.Flags().StringVarP(&opts.Author, "author", "A", "", "Filter by author")
	cmd.Flags().StringVarP(&opts.Category, "category", "c", "", "Filter by category name or slug")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by label")
	cmdutil.StringEnumFlag(cmd, &opts.State, "state", "s", stateOpen, []string{stateOpen, stateClosed, stateAll}, "Filter by state")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", defaultLimit, "Maximum number of discussions to fetch")
	cmdutil.NilBoolFlag(cmd, &opts.Answered, "answered", "", "Filter by answered state")
	cmdutil.StringEnumFlag(cmd, &opts.Sort, "sort", "", sortUpdated, []string{sortCreated, sortUpdated}, "Sort by field")
	cmdutil.StringEnumFlag(cmd, &opts.Order, "order", "", orderDesc, []string{orderAsc, orderDesc}, "Order of results")
	cmd.Flags().StringVarP(&opts.Search, "search", "S", "", "Search discussions with `query`")
	cmd.Flags().StringVar(&opts.After, "after", "", "Cursor for the next page of results")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List discussions in the web browser")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, discussionListFields)

	return cmd
}

// toFilterState maps CLI state strings to domain-level filter state pointers.
// "all" maps to nil (no state filter).
func toFilterState(v string) *string {
	switch v {
	case stateOpen:
		s := client.FilterStateOpen
		return &s
	case stateClosed:
		s := client.FilterStateClosed
		return &s
	default:
		return nil
	}
}

func toOrderByAndDirection(sort, order string) (string, string) {
	var orderBy string
	switch sort {
	case sortCreated:
		orderBy = client.OrderByCreated
	case sortUpdated:
		orderBy = client.OrderByUpdated
	default:
		orderBy = sort
	}

	var direction string
	switch order {
	case orderAsc:
		direction = client.OrderDirectionAsc
	case orderDesc:
		direction = client.OrderDirectionDesc
	default:
		direction = order
	}

	return orderBy, direction
}

func listRun(opts *ListOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.WebMode {
		return openInBrowser(opts, repo)
	}

	dc, err := opts.Client()
	if err != nil {
		return err
	}

	var categoryID string
	var categorySlug string
	if opts.Category != "" {
		categories, err := dc.ListCategories(repo)
		if err != nil {
			return err
		}
		cat, err := shared.MatchCategory(opts.Category, categories)
		if err != nil {
			return err
		}
		categoryID = cat.ID
		categorySlug = cat.Slug
	}

	state := toFilterState(opts.State)
	orderBy, direction := toOrderByAndDirection(opts.Sort, opts.Order)

	var result *client.DiscussionListResult

	useSearch := opts.Author != "" || len(opts.Labels) > 0 || opts.Search != ""
	if useSearch {
		filters := client.SearchFilters{
			Author:    opts.Author,
			Labels:    opts.Labels,
			State:     state,
			Category:  categorySlug,
			Answered:  opts.Answered,
			Keywords:  opts.Search,
			OrderBy:   orderBy,
			Direction: direction,
		}
		result, err = dc.Search(repo, filters, opts.After, opts.Limit)
	} else {
		filters := client.ListFilters{
			State:      state,
			CategoryID: categoryID,
			Answered:   opts.Answered,
			OrderBy:    orderBy,
			Direction:  direction,
		}
		result, err = dc.List(repo, filters, opts.After, opts.Limit)
	}
	if err != nil {
		return err
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, result)
	}

	if len(result.Discussions) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("no discussions found in %s", ghrepo.FullName(repo)))
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	printDiscussions(opts, ghrepo.FullName(repo), result.Discussions, result.TotalCount)
	return nil
}

func openInBrowser(opts *ListOptions, repo ghrepo.Interface) error {
	discussionsURL := ghrepo.GenerateRepoURL(repo, "discussions")

	var queryParts []string
	if opts.Search != "" {
		queryParts = append(queryParts, opts.Search)
	}
	if opts.State != "" && opts.State != stateAll {
		queryParts = append(queryParts, "is:"+opts.State)
	}
	if opts.Author != "" {
		queryParts = append(queryParts, fmt.Sprintf("author:%q", opts.Author))
	}
	for _, l := range opts.Labels {
		queryParts = append(queryParts, fmt.Sprintf("label:%q", l))
	}
	if opts.Category != "" {
		queryParts = append(queryParts, fmt.Sprintf("category:%q", opts.Category))
	}
	if opts.Answered != nil {
		if *opts.Answered {
			queryParts = append(queryParts, "is:answered")
		} else {
			queryParts = append(queryParts, "is:unanswered")
		}
	}

	if len(queryParts) > 0 {
		discussionsURL += "?" + url.Values{"q": {strings.Join(queryParts, " ")}}.Encode()
	}

	if opts.IO.IsStderrTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(discussionsURL))
	}
	return opts.Browser.Browse(discussionsURL)
}

func listHeader(repoName string, count, total int, state string) string {
	switch state {
	case stateOpen:
		return fmt.Sprintf("Showing %d of %d open discussions in %s", count, total, repoName)
	case stateClosed:
		return fmt.Sprintf("Showing %d of %d closed discussions in %s", count, total, repoName)
	default:
		return fmt.Sprintf("Showing %d of %d discussions in %s", count, total, repoName)
	}
}

func printDiscussions(opts *ListOptions, repoName string, discussions []client.Discussion, totalCount int) {
	isTerminal := opts.IO.IsStdoutTTY()
	cs := opts.IO.ColorScheme()
	now := opts.Now()

	if isTerminal {
		title := listHeader(repoName, len(discussions), totalCount, opts.State)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	headers := []string{"ID", "TITLE", "CATEGORY", "LABELS", "ANSWERED", "UPDATED"}
	if !isTerminal {
		headers = []string{"ID", "STATE", "TITLE", "CATEGORY", "LABELS", "ANSWERED", "UPDATED"}
	}
	tp := tableprinter.New(opts.IO, tableprinter.WithHeader(headers...))

	for _, d := range discussions {
		if isTerminal {
			idColor := cs.Green
			if d.Closed {
				idColor = cs.Muted
			}
			tp.AddField(fmt.Sprintf("#%d", d.Number), tableprinter.WithColor(idColor))
		} else {
			tp.AddField(fmt.Sprintf("%d", d.Number))
			if d.Closed {
				tp.AddField("CLOSED")
			} else {
				tp.AddField("OPEN")
			}
		}

		tp.AddField(text.RemoveExcessiveWhitespace(d.Title))
		tp.AddField(d.Category.Name)

		labelNames := make([]string, len(d.Labels))
		for i, l := range d.Labels {
			if isTerminal {
				labelNames[i] = cs.Label(l.Color, l.Name)
			} else {
				labelNames[i] = l.Name
			}
		}
		tp.AddField(strings.Join(labelNames, ", "), tableprinter.WithTruncate(nil))

		if d.Answered {
			if isTerminal {
				tp.AddField(cs.SuccessIcon())
			} else {
				tp.AddField("answered")
			}
		} else {
			tp.AddField("")
		}

		tp.AddTimeField(now, d.UpdatedAt, cs.Muted)
		tp.EndRow()
	}

	_ = tp.Render()

	if remaining := totalCount - len(discussions); isTerminal && remaining > 0 {
		fmt.Fprintf(opts.IO.Out, cs.Muted("And %d more\n"), remaining)
	}
}
