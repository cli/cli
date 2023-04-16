package status

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/cli/cli/v2/utils"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type hostConfig interface {
	DefaultHost() (string, string)
}

type StatusOptions struct {
	HttpClient   func() (*http.Client, error)
	HostConfig   hostConfig
	CachedClient func(*http.Client, time.Duration) *http.Client
	IO           *iostreams.IOStreams
	Org          string
	Exclude      []string
}

func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		CachedClient: func(c *http.Client, ttl time.Duration) *http.Client {
			return api.NewCachedHTTPClient(c, ttl)
		},
	}
	opts.HttpClient = f.HttpClient
	opts.IO = f.IOStreams
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print information about relevant issues, pull requests, and notifications across repositories",
		Long: heredoc.Doc(`
			The status command prints information about your work on GitHub across all the repositories you're subscribed to, including:

			- Assigned Issues
			- Assigned Pull Requests
			- Review Requests
			- Mentions
			- Repository Activity (new issues/pull requests, comments)
		`),
		Example: heredoc.Doc(`
			$ gh status -e cli/cli -e cli/go-gh # Exclude multiple repositories
			$ gh status -o cli # Limit results to a single organization
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := f.Config()
			if err != nil {
				return err
			}

			opts.HostConfig = cfg.Authentication()

			if runF != nil {
				return runF(opts)
			}

			return statusRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "Report status within an organization")
	cmd.Flags().StringSliceVarP(&opts.Exclude, "exclude", "e", []string{}, "Comma separated list of repos to exclude in owner/name format")

	return cmd
}

type Notification struct {
	Reason  string
	Subject struct {
		Title            string
		LatestCommentURL string `json:"latest_comment_url"`
		URL              string
		Type             string
	}
	Repository struct {
		Owner struct {
			Login string
		}
		FullName string `json:"full_name"`
	}
	index int
}

type StatusItem struct {
	Repository string // owner/repo
	Identifier string // eg cli/cli#1234 or just 1234
	preview    string // eg This is the truncated body of something...
	Reason     string // only used in repo activity
	index      int
}

func (s StatusItem) Preview() string {
	return strings.ReplaceAll(strings.ReplaceAll(s.preview, "\r", ""), "\n", " ")
}

type IssueOrPR struct {
	Number int
	Title  string
}

type Event struct {
	Type string
	Org  struct {
		Login string
	}
	CreatedAt time.Time `json:"created_at"`
	Repo      struct {
		Name string // owner/repo
	}
	Payload struct {
		Action      string
		Issue       IssueOrPR
		PullRequest IssueOrPR `json:"pull_request"`
		Comment     struct {
			Body    string
			HTMLURL string `json:"html_url"`
		}
	}
}

type SearchResult struct {
	Type       string `json:"__typename"`
	UpdatedAt  time.Time
	Title      string
	Number     int
	Repository struct {
		NameWithOwner string
	}
}

type Results []SearchResult

func (rs Results) Len() int {
	return len(rs)
}

func (rs Results) Less(i, j int) bool {
	return rs[i].UpdatedAt.After(rs[j].UpdatedAt)
}

func (rs Results) Swap(i, j int) {
	rs[i], rs[j] = rs[j], rs[i]
}

type stringSet interface {
	Len() int
	Add(string)
	ToSlice() []string
}

type StatusGetter struct {
	Client         *http.Client
	cachedClient   func(*http.Client, time.Duration) *http.Client
	host           string
	Org            string
	Exclude        []string
	AssignedPRs    []StatusItem
	AssignedIssues []StatusItem
	Mentions       []StatusItem
	ReviewRequests []StatusItem
	RepoActivity   []StatusItem

	authErrors   stringSet
	authErrorsMu sync.Mutex

	currentUsername string
	usernameMu      sync.Mutex
}

func NewStatusGetter(client *http.Client, hostname string, opts *StatusOptions) *StatusGetter {
	return &StatusGetter{
		Client:       client,
		Org:          opts.Org,
		Exclude:      opts.Exclude,
		cachedClient: opts.CachedClient,
		host:         hostname,
	}
}

func (s *StatusGetter) hostname() string {
	return s.host
}

func (s *StatusGetter) CachedClient(ttl time.Duration) *http.Client {
	return s.cachedClient(s.Client, ttl)
}

func (s *StatusGetter) ShouldExclude(repo string) bool {
	for _, exclude := range s.Exclude {
		if repo == exclude {
			return true
		}
	}
	return false
}

func (s *StatusGetter) CurrentUsername() (string, error) {
	s.usernameMu.Lock()
	defer s.usernameMu.Unlock()

	if s.currentUsername != "" {
		return s.currentUsername, nil
	}

	cachedClient := s.CachedClient(time.Hour * 48)
	cachingAPIClient := api.NewClientFromHTTP(cachedClient)
	currentUsername, err := api.CurrentLoginName(cachingAPIClient, s.hostname())
	if err != nil {
		return "", fmt.Errorf("failed to get current username: %w", err)
	}

	s.currentUsername = currentUsername
	return currentUsername, nil
}

func (s *StatusGetter) ActualMention(commentURL string) (string, error) {
	currentUsername, err := s.CurrentUsername()
	if err != nil {
		return "", err
	}

	// long cache period since once a comment is looked up, it never needs to be
	// consulted again.
	cachedClient := s.CachedClient(time.Hour * 24 * 30)
	c := api.NewClientFromHTTP(cachedClient)
	resp := struct {
		Body string
	}{}
	if err := c.REST(s.hostname(), "GET", commentURL, nil, &resp); err != nil {
		return "", err
	}

	if strings.Contains(resp.Body, "@"+currentUsername) {
		return resp.Body, nil
	}

	return "", nil
}

// These are split up by endpoint since it is along that boundary we parallelize
// work

// Populate .Mentions
func (s *StatusGetter) LoadNotifications() error {
	perPage := 100
	c := api.NewClientFromHTTP(s.Client)
	query := url.Values{}
	query.Add("per_page", fmt.Sprintf("%d", perPage))
	query.Add("participating", "true")
	query.Add("all", "true")

	fetchWorkers := 10
	ctx, abortFetching := context.WithCancel(context.Background())
	defer abortFetching()
	toFetch := make(chan Notification)
	fetched := make(chan StatusItem)

	wg := new(errgroup.Group)
	for i := 0; i < fetchWorkers; i++ {
		wg.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case n, ok := <-toFetch:
					if !ok {
						return nil
					}
					if actual, err := s.ActualMention(n.Subject.LatestCommentURL); actual != "" && err == nil {
						// I'm so sorry
						split := strings.Split(n.Subject.URL, "/")
						fetched <- StatusItem{
							Repository: n.Repository.FullName,
							Identifier: fmt.Sprintf("%s#%s", n.Repository.FullName, split[len(split)-1]),
							preview:    actual,
							index:      n.index,
						}
					} else if err != nil {
						var httpErr api.HTTPError
						if errors.As(err, &httpErr) && httpErr.StatusCode == 403 {
							s.addAuthError(httpErr.Message, factory.SSOURL())
						} else {
							abortFetching()
							return fmt.Errorf("could not fetch comment: %w", err)
						}
					}
				}
			}
		})
	}

	doneCh := make(chan struct{})
	go func() {
		for item := range fetched {
			s.Mentions = append(s.Mentions, item)
		}
		close(doneCh)
	}()

	// this sucks, having to fetch so much :/ but it was the only way in my
	// testing to really get enough mentions. I would love to be able to just
	// filter for mentions but it does not seem like the notifications API can
	// do that. I'd switch to the GraphQL version, but to my knowledge that does
	// not work with PATs right now.
	nIndex := 0
	p := fmt.Sprintf("notifications?%s", query.Encode())
	for pages := 0; pages < 3; pages++ {
		var resp []Notification
		next, err := c.RESTWithNext(s.hostname(), "GET", p, nil, &resp)
		if err != nil {
			var httpErr api.HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
				return fmt.Errorf("could not get notifications: %w", err)
			}
		}

		for _, n := range resp {
			if n.Reason != "mention" {
				continue
			}
			if s.Org != "" && n.Repository.Owner.Login != s.Org {
				continue
			}
			if s.ShouldExclude(n.Repository.FullName) {
				continue
			}
			n.index = nIndex
			nIndex++
			toFetch <- n
		}

		if next == "" || len(resp) < perPage {
			break
		}
		p = next
	}

	close(toFetch)
	err := wg.Wait()
	close(fetched)
	<-doneCh
	sort.Slice(s.Mentions, func(i, j int) bool {
		return s.Mentions[i].index < s.Mentions[j].index
	})
	return err
}

const searchQuery = `
fragment issue on Issue {
	__typename
	updatedAt
	title
	number
	repository {
		nameWithOwner
	}
}
fragment pr on PullRequest {
	__typename
	updatedAt
	title
	number
	repository {
		nameWithOwner
	}
}
query AssignedSearch($searchAssigns: String!, $searchReviews: String!, $limit: Int = 25) {
	assignments: search(first: $limit, type: ISSUE, query: $searchAssigns) {
		nodes {
			...issue
			...pr
		}
	}
	reviewRequested: search(first: $limit, type: ISSUE, query: $searchReviews) {
		nodes {
			...pr
		}
	}
}`

// Populate .AssignedPRs, .AssignedIssues, .ReviewRequests
func (s *StatusGetter) LoadSearchResults() error {
	c := api.NewClientFromHTTP(s.Client)

	searchAssigns := `assignee:@me state:open archived:false`
	searchReviews := `review-requested:@me state:open archived:false`
	if s.Org != "" {
		searchAssigns += " org:" + s.Org
		searchReviews += " org:" + s.Org
	}
	for _, repo := range s.Exclude {
		searchAssigns += " -repo:" + repo
		searchReviews += " -repo:" + repo
	}
	variables := map[string]interface{}{
		"searchAssigns": searchAssigns,
		"searchReviews": searchReviews,
	}

	var resp struct {
		Assignments struct {
			Nodes []*SearchResult
		}
		ReviewRequested struct {
			Nodes []*SearchResult
		}
	}
	err := c.GraphQL(s.hostname(), searchQuery, variables, &resp)
	if err != nil {
		var gqlErrResponse api.GraphQLError
		if errors.As(err, &gqlErrResponse) {
			gqlErrors := make([]ghAPI.GraphQLErrorItem, 0, len(gqlErrResponse.Errors))
			// Exclude any FORBIDDEN errors and show status for what we can.
			for _, gqlErr := range gqlErrResponse.Errors {
				if gqlErr.Type == "FORBIDDEN" {
					s.addAuthError(gqlErr.Message, factory.SSOURL())
				} else {
					gqlErrors = append(gqlErrors, gqlErr)
				}
			}
			if len(gqlErrors) == 0 {
				err = nil
			} else {
				err = api.GraphQLError{
					GraphQLError: &ghAPI.GraphQLError{
						Errors: gqlErrors,
					},
				}
			}
		}
		if err != nil {
			return fmt.Errorf("could not search for assignments: %w", err)
		}
	}

	prs := []SearchResult{}
	issues := []SearchResult{}
	reviewRequested := []SearchResult{}

	for _, node := range resp.Assignments.Nodes {
		if node == nil {
			continue // likely due to FORBIDDEN results
		}
		switch node.Type {
		case "Issue":
			issues = append(issues, *node)
		case "PullRequest":
			prs = append(prs, *node)
		default:
			return fmt.Errorf("unkown Assignments type: %q", node.Type)
		}
	}

	for _, node := range resp.ReviewRequested.Nodes {
		if node == nil {
			continue // likely due to FORBIDDEN results
		}
		reviewRequested = append(reviewRequested, *node)
	}

	sort.Sort(Results(issues))
	sort.Sort(Results(prs))
	sort.Sort(Results(reviewRequested))

	s.AssignedIssues = []StatusItem{}
	s.AssignedPRs = []StatusItem{}
	s.ReviewRequests = []StatusItem{}

	for _, i := range issues {
		s.AssignedIssues = append(s.AssignedIssues, StatusItem{
			Repository: i.Repository.NameWithOwner,
			Identifier: fmt.Sprintf("%s#%d", i.Repository.NameWithOwner, i.Number),
			preview:    i.Title,
		})
	}

	for _, pr := range prs {
		s.AssignedPRs = append(s.AssignedPRs, StatusItem{
			Repository: pr.Repository.NameWithOwner,
			Identifier: fmt.Sprintf("%s#%d", pr.Repository.NameWithOwner, pr.Number),
			preview:    pr.Title,
		})
	}

	for _, r := range reviewRequested {
		s.ReviewRequests = append(s.ReviewRequests, StatusItem{
			Repository: r.Repository.NameWithOwner,
			Identifier: fmt.Sprintf("%s#%d", r.Repository.NameWithOwner, r.Number),
			preview:    r.Title,
		})
	}

	return nil
}

// Populate .RepoActivity
func (s *StatusGetter) LoadEvents() error {
	perPage := 100
	c := api.NewClientFromHTTP(s.Client)
	query := url.Values{}
	query.Add("per_page", fmt.Sprintf("%d", perPage))

	currentUsername, err := s.CurrentUsername()
	if err != nil {
		return err
	}

	var events []Event
	var resp []Event
	pages := 0
	p := fmt.Sprintf("users/%s/received_events?%s", currentUsername, query.Encode())
	for pages < 2 {
		next, err := c.RESTWithNext(s.hostname(), "GET", p, nil, &resp)
		if err != nil {
			var httpErr api.HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
				return fmt.Errorf("could not get events: %w", err)
			}
		}
		events = append(events, resp...)
		if next == "" || len(resp) < perPage {
			break
		}

		pages++
		p = next
	}

	s.RepoActivity = []StatusItem{}

	for _, e := range events {
		if s.Org != "" && e.Org.Login != s.Org {
			continue
		}
		if s.ShouldExclude(e.Repo.Name) {
			continue
		}
		si := StatusItem{}
		var number int
		switch e.Type {
		case "IssuesEvent":
			if e.Payload.Action != "opened" {
				continue
			}
			si.Reason = "new issue"
			si.preview = e.Payload.Issue.Title
			number = e.Payload.Issue.Number
		case "PullRequestEvent":
			if e.Payload.Action != "opened" {
				continue
			}
			si.Reason = "new PR"
			si.preview = e.Payload.PullRequest.Title
			number = e.Payload.PullRequest.Number
		case "PullRequestReviewCommentEvent":
			si.Reason = "comment on " + e.Payload.PullRequest.Title
			si.preview = e.Payload.Comment.Body
			number = e.Payload.PullRequest.Number
		case "IssueCommentEvent":
			si.Reason = "comment on " + e.Payload.Issue.Title
			si.preview = e.Payload.Comment.Body
			number = e.Payload.Issue.Number
		default:
			continue
		}
		si.Repository = e.Repo.Name
		si.Identifier = fmt.Sprintf("%s#%d", e.Repo.Name, number)
		s.RepoActivity = append(s.RepoActivity, si)
	}

	return nil
}

func (s *StatusGetter) addAuthError(msg, ssoURL string) {
	s.authErrorsMu.Lock()
	defer s.authErrorsMu.Unlock()

	if s.authErrors == nil {
		s.authErrors = set.NewStringSet()
	}

	if ssoURL != "" {
		s.authErrors.Add(fmt.Sprintf("%s\nAuthorize in your web browser:  %s", msg, ssoURL))
	} else {
		s.authErrors.Add(msg)
	}
}

func (s *StatusGetter) HasAuthErrors() bool {
	s.authErrorsMu.Lock()
	defer s.authErrorsMu.Unlock()

	return s.authErrors != nil && s.authErrors.Len() > 0
}

func statusRun(opts *StatusOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create client: %w", err)
	}

	hostname, _ := opts.HostConfig.DefaultHost()

	sg := NewStatusGetter(client, hostname, opts)

	// TODO break out sections into individual subcommands

	g := new(errgroup.Group)
	g.Go(func() error {
		if err := sg.LoadNotifications(); err != nil {
			return fmt.Errorf("could not load notifications: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := sg.LoadEvents(); err != nil {
			return fmt.Errorf("could not load events: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := sg.LoadSearchResults(); err != nil {
			return fmt.Errorf("failed to search: %w", err)
		}
		return nil
	})

	opts.IO.StartProgressIndicator()
	err = g.Wait()
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	out := opts.IO.Out
	fullWidth := opts.IO.TerminalWidth()
	halfWidth := (fullWidth / 2) - 2

	idStyle := cs.Cyan
	leftHalfStyle := lipgloss.NewStyle().Width(halfWidth).Padding(0).MarginRight(1).BorderRight(true).BorderStyle(lipgloss.NormalBorder())
	rightHalfStyle := lipgloss.NewStyle().Width(halfWidth).Padding(0)

	section := func(header string, items []StatusItem, width, rowLimit int) (string, error) {
		tableOut := &bytes.Buffer{}
		fmt.Fprintln(tableOut, cs.Bold(header))
		//nolint:staticcheck // SA1019: utils.NewTablePrinterWithOptions is deprecated: use internal/tableprinter
		tp := utils.NewTablePrinterWithOptions(opts.IO, utils.TablePrinterOptions{
			IsTTY:    opts.IO.IsStdoutTTY(),
			MaxWidth: width,
			Out:      tableOut,
		})
		if len(items) == 0 {
			tp.AddField("Nothing here ^_^", nil, nil)
			tp.EndRow()
		} else {
			for i, si := range items {
				if i == rowLimit {
					break
				}
				tp.AddField(si.Identifier, nil, idStyle)
				if si.Reason != "" {
					tp.AddField(si.Reason, nil, nil)
				}
				tp.AddField(si.Preview(), nil, nil)
				tp.EndRow()
			}
		}

		err := tp.Render()
		if err != nil {
			return "", err
		}

		return tableOut.String(), nil
	}

	mSection, err := section("Mentions", sg.Mentions, halfWidth, 5)
	if err != nil {
		return fmt.Errorf("failed to render 'Mentions': %w", err)
	}
	mSection = rightHalfStyle.Render(mSection)

	rrSection, err := section("Review Requests", sg.ReviewRequests, halfWidth, 5)
	if err != nil {
		return fmt.Errorf("failed to render 'Review Requests': %w", err)
	}
	rrSection = leftHalfStyle.Render(rrSection)

	prSection, err := section("Assigned Pull Requests", sg.AssignedPRs, halfWidth, 5)
	if err != nil {
		return fmt.Errorf("failed to render 'Assigned Pull Requests': %w", err)
	}
	prSection = rightHalfStyle.Render(prSection)

	issueSection, err := section("Assigned Issues", sg.AssignedIssues, halfWidth, 5)
	if err != nil {
		return fmt.Errorf("failed to render 'Assigned Issues': %w", err)
	}
	issueSection = leftHalfStyle.Render(issueSection)

	raSection, err := section("Repository Activity", sg.RepoActivity, fullWidth, 10)
	if err != nil {
		return fmt.Errorf("failed to render 'Repository Activity': %w", err)
	}

	fmt.Fprintln(out, lipgloss.JoinHorizontal(lipgloss.Top, issueSection, prSection))
	fmt.Fprintln(out, lipgloss.JoinHorizontal(lipgloss.Top, rrSection, mSection))
	fmt.Fprintln(out, raSection)

	if sg.HasAuthErrors() {
		errs := sg.authErrors.ToSlice()
		sort.Strings(errs)
		for _, msg := range errs {
			fmt.Fprintln(out, cs.Gray(fmt.Sprintf("warning: %s", msg)))
		}
	}

	return nil
}
