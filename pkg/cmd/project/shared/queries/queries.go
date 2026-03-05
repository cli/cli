package queries

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
)

func NewClient(httpClient *http.Client, hostname string, ios *iostreams.IOStreams) *Client {
	apiClient := &hostScopedClient{
		hostname: hostname,
		Client:   api.NewClientFromHTTP(httpClient),
	}
	return &Client{
		apiClient: apiClient,
		io:        ios,
		prompter:  prompter.New("", ios),
	}
}

// TestClientOpt is a test option for the test client.
type TestClientOpt func(*Client)

// WithPrompter is a test option to set the prompter for the test client.
func WithPrompter(p iprompter) TestClientOpt {
	return func(c *Client) {
		c.prompter = p
	}
}

func NewTestClient(opts ...TestClientOpt) *Client {
	apiClient := &hostScopedClient{
		hostname: "github.com",
		Client:   api.NewClientFromHTTP(http.DefaultClient),
	}
	io, _, _, _ := iostreams.Test()
	c := &Client{
		apiClient: apiClient,
		io:        io,
		prompter:  nil,
	}

	for _, o := range opts {
		o(c)
	}
	return c
}

type iprompter interface {
	Select(string, string, []string) (int, error)
}

type hostScopedClient struct {
	*api.Client
	hostname string
}

func (c *hostScopedClient) Query(queryName string, query interface{}, variables map[string]interface{}) error {
	return c.Client.Query(c.hostname, queryName, query, variables)
}

func (c *hostScopedClient) Mutate(queryName string, query interface{}, variables map[string]interface{}) error {
	return c.Client.Mutate(c.hostname, queryName, query, variables)
}

type graphqlClient interface {
	Query(queryName string, query interface{}, variables map[string]interface{}) error
	Mutate(queryName string, query interface{}, variables map[string]interface{}) error
}

type Client struct {
	apiClient graphqlClient
	io        *iostreams.IOStreams
	prompter  iprompter
}

const (
	LimitDefault = 30
	LimitMax     = 100 // https://docs.github.com/en/graphql/overview/resource-limitations#node-limit
)

// doQueryWithProgressIndicator wraps API calls with a progress indicator.
// The query name is used in the progress indicator label.
func (c *Client) doQueryWithProgressIndicator(name string, query interface{}, variables map[string]interface{}) error {
	c.io.StartProgressIndicatorWithLabel(fmt.Sprintf("Fetching %s", name))
	defer c.io.StopProgressIndicator()
	err := c.apiClient.Query(name, query, variables)
	return handleError(err)
}

// TODO: un-export this since it couples the caller heavily to api.GraphQLClient
func (c *Client) Mutate(operationName string, query interface{}, variables map[string]interface{}) error {
	err := c.apiClient.Mutate(operationName, query, variables)
	return handleError(err)
}

func (c *Client) Query(operationName string, query interface{}, variables map[string]interface{}) error {
	err := c.apiClient.Query(operationName, query, variables)
	return handleError(err)
}

// PageInfo is a PageInfo GraphQL object https://docs.github.com/en/graphql/reference/objects#pageinfo.
type PageInfo struct {
	EndCursor   githubv4.String
	HasNextPage bool
}

// Project is a ProjectV2 GraphQL object https://docs.github.com/en/graphql/reference/objects#projectv2.
type Project struct {
	Number           int32
	URL              string
	ShortDescription string
	Public           bool
	Closed           bool
	// The Template field is commented out due to https://github.com/cli/cli/issues/8103.
	// We released gh v2.34.0 without realizing the Template field does not exist
	// on GHES 3.8 and older. This broke all project commands for users targeting GHES 3.8
	// and older. In order to fix this we will no longer query the Template field until
	// GHES 3.8 gets deprecated on 2024-03-07. This solution was simpler and quicker
	// than adding a feature detection measure to every place this query is used.
	// It does have the negative consequence that we have had to remove the
	// Template field when outputting projects to JSON using the --format flag supported
	// by a number of project commands. See `pkg/cmd/project/shared/format/json.go` for
	// implementation.
	// Template         bool
	Title  string
	ID     string
	Readme string
	Items  struct {
		PageInfo   PageInfo
		TotalCount int
		Nodes      []ProjectItem
	} `graphql:"items(first: $firstItems, after: $afterItems, query: $query)"`
	Fields ProjectFields `graphql:"fields(first: $firstFields, after: $afterFields)"`
	Owner  struct {
		TypeName string `graphql:"__typename"`
		User     struct {
			Login string
		} `graphql:"... on User"`
		Organization struct {
			Login string
		} `graphql:"... on Organization"`
	}
}

// ProjectMutationQuery is a ProjectV2 response shape for mutation payloads.
// It intentionally avoids the queryable items connection to prevent requiring a $query variable.
type ProjectMutationQuery struct {
	Number           int32
	URL              string
	ShortDescription string
	Public           bool
	Closed           bool
	Title            string
	ID               string
	Readme           string
	Items            struct {
		TotalCount int
	} `graphql:"items(first: $firstItems, after: $afterItems)"`
	Fields struct {
		TotalCount int
	} `graphql:"fields(first: $firstFields, after: $afterFields)"`
	Owner struct {
		TypeName string `graphql:"__typename"`
		User     struct {
			Login string
		} `graphql:"... on User"`
		Organization struct {
			Login string
		} `graphql:"... on Organization"`
	}
}

// Below, you will find the query structs to represent fetching a project via the GraphQL API.
// Prior to GHES 3.20, the query argument did not exist on the items connection, so we have
// one base struct and two structs that embed and add the Items connection with and without the query argument.
// The expectation is that these will then be converted into the Project domain struct above.
type projectQueryBase struct {
	Number           int32
	URL              string
	ShortDescription string
	Public           bool
	Closed           bool
	Title            string
	ID               string
	Readme           string
	Owner            struct {
		TypeName string `graphql:"__typename"`
		User     struct {
			Login string
		} `graphql:"... on User"`
		Organization struct {
			Login string
		} `graphql:"... on Organization"`
	}
	Fields ProjectFields `graphql:"fields(first: $firstFields, after: $afterFields)"`
}

type projectQueryWithQueryableItems struct {
	projectQueryBase
	Items struct {
		PageInfo   PageInfo
		TotalCount int
		Nodes      []ProjectItem
	} `graphql:"items(first: $firstItems, after: $afterItems, query: $query)"`
}

type projectQueryWithoutQueryableItems struct {
	projectQueryBase
	Items struct {
		PageInfo   PageInfo
		TotalCount int
		Nodes      []ProjectItem
	} `graphql:"items(first: $firstItems, after: $afterItems)"`
}

func newProjectFromQueryBase(source projectQueryBase) *Project {
	project := &Project{
		Number:           source.Number,
		URL:              source.URL,
		ShortDescription: source.ShortDescription,
		Public:           source.Public,
		Closed:           source.Closed,
		Title:            source.Title,
		ID:               source.ID,
		Readme:           source.Readme,
	}
	project.Owner.TypeName = source.Owner.TypeName
	project.Owner.User.Login = source.Owner.User.Login
	project.Owner.Organization.Login = source.Owner.Organization.Login
	project.Fields = source.Fields
	return project
}

func newProjectFromQueryWithItemsQuery(source projectQueryWithQueryableItems) *Project {
	project := newProjectFromQueryBase(source.projectQueryBase)
	project.Items.PageInfo = source.Items.PageInfo
	project.Items.TotalCount = source.Items.TotalCount
	project.Items.Nodes = source.Items.Nodes
	return project
}

func newProjectFromQueryWithoutItemsQuery(source projectQueryWithoutQueryableItems) *Project {
	project := newProjectFromQueryBase(source.projectQueryBase)
	project.Items.PageInfo = source.Items.PageInfo
	project.Items.TotalCount = source.Items.TotalCount
	project.Items.Nodes = source.Items.Nodes
	return project
}

func (p Project) DetailedItems() map[string]interface{} {
	return map[string]interface{}{
		"items":      serializeProjectWithItems(&p),
		"totalCount": p.Items.TotalCount,
	}
}

func (p Project) ExportData(_ []string) map[string]interface{} {
	return map[string]interface{}{
		"number":           p.Number,
		"url":              p.URL,
		"shortDescription": p.ShortDescription,
		"public":           p.Public,
		"closed":           p.Closed,
		"title":            p.Title,
		"id":               p.ID,
		"readme":           p.Readme,
		"items": map[string]interface{}{
			"totalCount": p.Items.TotalCount,
		},
		"fields": map[string]interface{}{
			"totalCount": p.Fields.TotalCount,
		},
		"owner": map[string]interface{}{
			"type":  p.OwnerType(),
			"login": p.OwnerLogin(),
		},
	}
}

func (p Project) OwnerType() string {
	return p.Owner.TypeName
}

func (p Project) OwnerLogin() string {
	if p.OwnerType() == "User" {
		return p.Owner.User.Login
	}
	return p.Owner.Organization.Login
}

func (p ProjectMutationQuery) ExportData(_ []string) map[string]interface{} {
	return map[string]interface{}{
		"number":           p.Number,
		"url":              p.URL,
		"shortDescription": p.ShortDescription,
		"public":           p.Public,
		"closed":           p.Closed,
		"title":            p.Title,
		"id":               p.ID,
		"readme":           p.Readme,
		"items": map[string]interface{}{
			"totalCount": p.Items.TotalCount,
		},
		"fields": map[string]interface{}{
			"totalCount": p.Fields.TotalCount,
		},
		"owner": map[string]interface{}{
			"type":  p.OwnerType(),
			"login": p.OwnerLogin(),
		},
	}
}

func (p ProjectMutationQuery) OwnerType() string {
	return p.Owner.TypeName
}

func (p ProjectMutationQuery) OwnerLogin() string {
	if p.OwnerType() == "User" {
		return p.Owner.User.Login
	}
	return p.Owner.Organization.Login
}

type Projects struct {
	Nodes      []Project
	TotalCount int
}

func (p Projects) ExportData(_ []string) map[string]interface{} {
	v := make([]map[string]interface{}, len(p.Nodes))
	for i := range p.Nodes {
		v[i] = p.Nodes[i].ExportData(nil)
	}
	return map[string]interface{}{
		"projects":   v,
		"totalCount": p.TotalCount,
	}
}

// ProjectItem is a ProjectV2Item GraphQL object https://docs.github.com/en/graphql/reference/objects#projectv2item.
type ProjectItem struct {
	Content     ProjectItemContent
	Id          string
	FieldValues struct {
		Nodes []FieldValueNodes
	} `graphql:"fieldValues(first: 100)"` // hardcoded to 100 for now on the assumption that this is a reasonable limit
}

type ProjectItemContent struct {
	TypeName    string      `graphql:"__typename"`
	DraftIssue  DraftIssue  `graphql:"... on DraftIssue"`
	PullRequest PullRequest `graphql:"... on PullRequest"`
	Issue       Issue       `graphql:"... on Issue"`
}

type FieldValueNodes struct {
	Type                        string `graphql:"__typename"`
	ProjectV2ItemFieldDateValue struct {
		Date  string
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldDateValue"`
	ProjectV2ItemFieldIterationValue struct {
		Title       string
		StartDate   string
		Duration    int
		Field       ProjectField
		IterationId string
	} `graphql:"... on ProjectV2ItemFieldIterationValue"`
	ProjectV2ItemFieldLabelValue struct {
		Labels struct {
			Nodes []struct {
				Name string
			}
		} `graphql:"labels(first: 10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldLabelValue"`
	ProjectV2ItemFieldNumberValue struct {
		Number float64
		Field  ProjectField
	} `graphql:"... on ProjectV2ItemFieldNumberValue"`
	ProjectV2ItemFieldSingleSelectValue struct {
		Name  string
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
	ProjectV2ItemFieldTextValue struct {
		Text  string
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldTextValue"`
	ProjectV2ItemFieldMilestoneValue struct {
		Milestone struct {
			Title       string
			Description string
			DueOn       string
		}
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldMilestoneValue"`
	ProjectV2ItemFieldPullRequestValue struct {
		PullRequests struct {
			Nodes []struct {
				Url string
			}
		} `graphql:"pullRequests(first:10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldPullRequestValue"`
	ProjectV2ItemFieldRepositoryValue struct {
		Repository struct {
			Url string
		}
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldRepositoryValue"`
	ProjectV2ItemFieldUserValue struct {
		Users struct {
			Nodes []struct {
				Login string
			}
		} `graphql:"users(first: 10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldUserValue"`
	ProjectV2ItemFieldReviewerValue struct {
		Reviewers struct {
			Nodes []struct {
				Type string `graphql:"__typename"`
				Team struct {
					Name string
				} `graphql:"... on Team"`
				User struct {
					Login string
				} `graphql:"... on User"`
			}
		} `graphql:"reviewers(first: 10)"` // experienced issues with larger limits, 10 seems like enough for now
		Field ProjectField
	} `graphql:"... on ProjectV2ItemFieldReviewerValue"`
}

func (v FieldValueNodes) ID() string {
	switch v.Type {
	case "ProjectV2ItemFieldDateValue":
		return v.ProjectV2ItemFieldDateValue.Field.ID()
	case "ProjectV2ItemFieldIterationValue":
		return v.ProjectV2ItemFieldIterationValue.Field.ID()
	case "ProjectV2ItemFieldNumberValue":
		return v.ProjectV2ItemFieldNumberValue.Field.ID()
	case "ProjectV2ItemFieldSingleSelectValue":
		return v.ProjectV2ItemFieldSingleSelectValue.Field.ID()
	case "ProjectV2ItemFieldTextValue":
		return v.ProjectV2ItemFieldTextValue.Field.ID()
	case "ProjectV2ItemFieldMilestoneValue":
		return v.ProjectV2ItemFieldMilestoneValue.Field.ID()
	case "ProjectV2ItemFieldLabelValue":
		return v.ProjectV2ItemFieldLabelValue.Field.ID()
	case "ProjectV2ItemFieldPullRequestValue":
		return v.ProjectV2ItemFieldPullRequestValue.Field.ID()
	case "ProjectV2ItemFieldRepositoryValue":
		return v.ProjectV2ItemFieldRepositoryValue.Field.ID()
	case "ProjectV2ItemFieldUserValue":
		return v.ProjectV2ItemFieldUserValue.Field.ID()
	case "ProjectV2ItemFieldReviewerValue":
		return v.ProjectV2ItemFieldReviewerValue.Field.ID()
	}

	return ""
}

type DraftIssue struct {
	ID    string
	Body  string
	Title string
}

func (i DraftIssue) ExportData(_ []string) map[string]interface{} {
	v := map[string]interface{}{
		"title": i.Title,
		"body":  i.Body,
		"type":  "DraftIssue",
	}
	// Emulate omitempty.
	if i.ID != "" {
		v["id"] = i.ID
	}
	return v
}

type PullRequest struct {
	Body       string
	Title      string
	Number     int
	URL        string
	Repository struct {
		NameWithOwner string
	}
}

func (pr PullRequest) ExportData(_ []string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "PullRequest",
		"body":       pr.Body,
		"title":      pr.Title,
		"number":     pr.Number,
		"repository": pr.Repository.NameWithOwner,
		"url":        pr.URL,
	}
}

type Issue struct {
	Body       string
	Title      string
	Number     int
	URL        string
	Repository struct {
		NameWithOwner string
	}
}

func (i Issue) ExportData(_ []string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "Issue",
		"body":       i.Body,
		"title":      i.Title,
		"number":     i.Number,
		"repository": i.Repository.NameWithOwner,
		"url":        i.URL,
	}
}

func (p ProjectItem) DetailedItem() exportable {
	switch p.Type() {
	case "DraftIssue":
		return DraftIssue{
			ID:    p.Content.DraftIssue.ID,
			Body:  p.Body(),
			Title: p.Title(),
		}

	case "Issue":
		return Issue{
			Body:   p.Body(),
			Title:  p.Title(),
			Number: p.Number(),
			Repository: struct{ NameWithOwner string }{
				NameWithOwner: p.Repo(),
			},
			URL: p.URL(),
		}

	case "PullRequest":
		return PullRequest{
			Body:   p.Body(),
			Title:  p.Title(),
			Number: p.Number(),
			Repository: struct{ NameWithOwner string }{
				NameWithOwner: p.Repo(),
			},
			URL: p.URL(),
		}
	}
	return nil
}

// Type is the underlying type of the project item.
func (p ProjectItem) Type() string {
	return p.Content.TypeName
}

// Title is the title of the project item.
func (p ProjectItem) Title() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Title
	case "PullRequest":
		return p.Content.PullRequest.Title
	case "DraftIssue":
		return p.Content.DraftIssue.Title
	}
	return ""
}

// Body is the body of the project item.
func (p ProjectItem) Body() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Body
	case "PullRequest":
		return p.Content.PullRequest.Body
	case "DraftIssue":
		return p.Content.DraftIssue.Body
	}
	return ""
}

// Number is the number of the project item. It is only valid for issues and pull requests.
func (p ProjectItem) Number() int {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Number
	case "PullRequest":
		return p.Content.PullRequest.Number
	}

	return 0
}

// ID is the id of the ProjectItem.
func (p ProjectItem) ID() string {
	return p.Id
}

// Repo is the repository of the project item. It is only valid for issues and pull requests.
func (p ProjectItem) Repo() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.Repository.NameWithOwner
	case "PullRequest":
		return p.Content.PullRequest.Repository.NameWithOwner
	}
	return ""
}

// URL is the URL of the project item. Note the draft issues do not have URLs
func (p ProjectItem) URL() string {
	switch p.Content.TypeName {
	case "Issue":
		return p.Content.Issue.URL
	case "PullRequest":
		return p.Content.PullRequest.URL
	}
	return ""
}

func (p ProjectItem) ExportData(_ []string) map[string]interface{} {
	v := map[string]interface{}{
		"id":    p.ID(),
		"title": p.Title(),
		"body":  p.Body(),
		"type":  p.Type(),
	}
	// Emulate omitempty.
	if url := p.URL(); url != "" {
		v["url"] = url
	}
	return v
}

// ProjectItems returns the items of a project. If the OwnerType is VIEWER, no login is required.
// If limit is 0, the default limit is used. The queryStr parameter is passed as a server-side
// filter to the items connection, using the same syntax as the GitHub Projects filter bar
// (e.g. "assignee:octocat", "status:done").
func (c *Client) ProjectItems(o *Owner, number int32, limit int, queryStr string) (*Project, error) {
	project := &Project{}
	if limit == 0 {
		limit = LimitDefault
	}

	// set first to the min of limit and LimitMax
	first := LimitMax
	if limit < first {
		first = limit
	}

	variables := map[string]interface{}{
		"firstItems":  githubv4.Int(first),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(LimitMax),
		"afterFields": (*githubv4.String)(nil),
		"number":      githubv4.Int(number),
	}
	if queryStr != "" {
		variables["query"] = githubv4.String(queryStr)
	}

	var query pager[ProjectItem]
	var queryName string
	switch o.Type {
	case UserOwner:
		variables["login"] = githubv4.String(o.Login)
		if queryStr == "" {
			query = &userOwnerWithItemsNoQuery{}
		} else {
			query = &userOwnerWithItems{}
		}
		queryName = "UserProjectWithItems"
	case OrgOwner:
		variables["login"] = githubv4.String(o.Login)
		if queryStr == "" {
			query = &orgOwnerWithItemsNoQuery{}
		} else {
			query = &orgOwnerWithItems{}
		}
		queryName = "OrgProjectWithItems"
	case ViewerOwner:
		if queryStr == "" {
			query = &viewerOwnerWithItemsNoQuery{}
		} else {
			query = &viewerOwnerWithItems{}
		}
		queryName = "ViewerProjectWithItems"
	}
	err := c.doQueryWithProgressIndicator(queryName, query, variables)
	if err != nil {
		return project, err
	}
	project = query.Project()

	items, err := paginateAttributes(c, query, variables, queryName, "firstItems", "afterItems", limit, query.Nodes())
	if err != nil {
		return project, err
	}

	project.Items.Nodes = items
	return project, nil
}

// pager is an interface for paginating over the attributes of a Project.
type pager[N projectAttribute] interface {
	HasNextPage() bool
	EndCursor() string
	Nodes() []N
	Project() *Project
}

// userOwnerWithItemsNoQuery
func (q userOwnerWithItemsNoQuery) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q userOwnerWithItemsNoQuery) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q userOwnerWithItemsNoQuery) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q userOwnerWithItemsNoQuery) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// userOwnerWithItems
func (q userOwnerWithItems) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q userOwnerWithItems) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q userOwnerWithItems) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q userOwnerWithItems) Project() *Project {
	return newProjectFromQueryWithItemsQuery(q.Owner.Project)
}

// orgOwnerWithItems
func (q orgOwnerWithItems) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q orgOwnerWithItems) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q orgOwnerWithItems) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q orgOwnerWithItems) Project() *Project {
	return newProjectFromQueryWithItemsQuery(q.Owner.Project)
}

// orgOwnerWithItemsNoQuery
func (q orgOwnerWithItemsNoQuery) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q orgOwnerWithItemsNoQuery) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q orgOwnerWithItemsNoQuery) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q orgOwnerWithItemsNoQuery) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// viewerOwnerWithItems
func (q viewerOwnerWithItems) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q viewerOwnerWithItems) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q viewerOwnerWithItems) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q viewerOwnerWithItems) Project() *Project {
	return newProjectFromQueryWithItemsQuery(q.Owner.Project)
}

// viewerOwnerWithItemsNoQuery
func (q viewerOwnerWithItemsNoQuery) HasNextPage() bool {
	return q.Owner.Project.Items.PageInfo.HasNextPage
}

func (q viewerOwnerWithItemsNoQuery) EndCursor() string {
	return string(q.Owner.Project.Items.PageInfo.EndCursor)
}

func (q viewerOwnerWithItemsNoQuery) Nodes() []ProjectItem {
	return q.Owner.Project.Items.Nodes
}

func (q viewerOwnerWithItemsNoQuery) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// userOwnerWithFields
func (q userOwnerWithFields) HasNextPage() bool {
	return q.Owner.Project.Fields.PageInfo.HasNextPage
}

func (q userOwnerWithFields) EndCursor() string {
	return string(q.Owner.Project.Fields.PageInfo.EndCursor)
}

func (q userOwnerWithFields) Nodes() []ProjectField {
	return q.Owner.Project.Fields.Nodes
}

func (q userOwnerWithFields) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// orgOwnerWithFields
func (q orgOwnerWithFields) HasNextPage() bool {
	return q.Owner.Project.Fields.PageInfo.HasNextPage
}

func (q orgOwnerWithFields) EndCursor() string {
	return string(q.Owner.Project.Fields.PageInfo.EndCursor)
}

func (q orgOwnerWithFields) Nodes() []ProjectField {
	return q.Owner.Project.Fields.Nodes
}

func (q orgOwnerWithFields) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

// viewerOwnerWithFields
func (q viewerOwnerWithFields) HasNextPage() bool {
	return q.Owner.Project.Fields.PageInfo.HasNextPage
}

func (q viewerOwnerWithFields) EndCursor() string {
	return string(q.Owner.Project.Fields.PageInfo.EndCursor)
}

func (q viewerOwnerWithFields) Nodes() []ProjectField {
	return q.Owner.Project.Fields.Nodes
}

func (q viewerOwnerWithFields) Project() *Project {
	return newProjectFromQueryWithoutItemsQuery(q.Owner.Project)
}

type projectAttribute interface {
	ProjectItem | ProjectField
}

// paginateAttributes is for paginating over the attributes of a project, such as items or fields
//
// firstKey and afterKey are the keys in the variables map that are used to set the first and after
// as these are set independently based on the attribute type, such as item or field.
//
// limit is the maximum number of attributes to return, or 0 for no limit.
//
// nodes is the list of attributes that have already been fetched.
//
// the return value is a slice of the newly fetched attributes appended to nodes.
func paginateAttributes[N projectAttribute](c *Client, p pager[N], variables map[string]any, queryName string, firstKey string, afterKey string, limit int, nodes []N) ([]N, error) {
	hasNextPage := p.HasNextPage()
	cursor := p.EndCursor()
	for {
		if !hasNextPage || len(nodes) >= limit {
			return nodes, nil
		}

		if len(nodes)+LimitMax > limit {
			first := limit - len(nodes)
			variables[firstKey] = githubv4.Int(first)
		}

		// set the cursor to the end of the last page
		variables[afterKey] = (*githubv4.String)(&cursor)
		err := c.doQueryWithProgressIndicator(queryName, p, variables)
		if err != nil {
			return nodes, err
		}

		nodes = append(nodes, p.Nodes()...)
		hasNextPage = p.HasNextPage()
		cursor = p.EndCursor()
	}
}

// ProjectField is a ProjectV2FieldConfiguration GraphQL object https://docs.github.com/en/graphql/reference/unions#projectv2fieldconfiguration.
type ProjectField struct {
	TypeName string `graphql:"__typename"`
	Field    struct {
		ID       string
		Name     string
		DataType string
	} `graphql:"... on ProjectV2Field"`
	IterationField struct {
		ID       string
		Name     string
		DataType string
	} `graphql:"... on ProjectV2IterationField"`
	SingleSelectField struct {
		ID       string
		Name     string
		DataType string
		Options  []SingleSelectFieldOptions
	} `graphql:"... on ProjectV2SingleSelectField"`
}

// ID is the ID of the project field.
func (p ProjectField) ID() string {
	if p.TypeName == "ProjectV2Field" {
		return p.Field.ID
	} else if p.TypeName == "ProjectV2IterationField" {
		return p.IterationField.ID
	} else if p.TypeName == "ProjectV2SingleSelectField" {
		return p.SingleSelectField.ID
	}
	return ""
}

// Name is the name of the project field.
func (p ProjectField) Name() string {
	if p.TypeName == "ProjectV2Field" {
		return p.Field.Name
	} else if p.TypeName == "ProjectV2IterationField" {
		return p.IterationField.Name
	} else if p.TypeName == "ProjectV2SingleSelectField" {
		return p.SingleSelectField.Name
	}
	return ""
}

// Type is the typename of the project field.
func (p ProjectField) Type() string {
	return p.TypeName
}

type SingleSelectFieldOptions struct {
	ID   string
	Name string
}

func (f SingleSelectFieldOptions) ExportData(_ []string) map[string]interface{} {
	return map[string]interface{}{
		"id":   f.ID,
		"name": f.Name,
	}
}

func (p ProjectField) Options() []SingleSelectFieldOptions {
	if p.TypeName == "ProjectV2SingleSelectField" {
		var options []SingleSelectFieldOptions
		for _, o := range p.SingleSelectField.Options {
			options = append(options, SingleSelectFieldOptions{
				ID:   o.ID,
				Name: o.Name,
			})
		}
		return options
	}
	return nil
}

func (p ProjectField) ExportData(_ []string) map[string]interface{} {
	v := map[string]interface{}{
		"id":   p.ID(),
		"name": p.Name(),
		"type": p.Type(),
	}
	// Emulate omitempty
	if opts := p.Options(); len(opts) != 0 {
		options := make([]map[string]interface{}, len(opts))
		for i, opt := range opts {
			options[i] = opt.ExportData(nil)
		}
		v["options"] = options
	}
	return v
}

type ProjectFields struct {
	TotalCount int
	Nodes      []ProjectField
	PageInfo   PageInfo
}

func (p ProjectFields) ExportData(_ []string) map[string]interface{} {
	fields := make([]map[string]interface{}, len(p.Nodes))
	for i := range p.Nodes {
		fields[i] = p.Nodes[i].ExportData(nil)
	}
	return map[string]interface{}{
		"fields":     fields,
		"totalCount": p.TotalCount,
	}
}

// ProjectFields returns a project with fields. If the OwnerType is VIEWER, no login is required.
// If limit is 0, the default limit is used.
func (c *Client) ProjectFields(o *Owner, number int32, limit int) (*Project, error) {
	project := &Project{}
	if limit == 0 {
		limit = LimitDefault
	}

	// set first to the min of limit and LimitMax
	first := LimitMax
	if limit < first {
		first = limit
	}
	variables := map[string]interface{}{
		"firstItems":  githubv4.Int(LimitMax),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(first),
		"afterFields": (*githubv4.String)(nil),
		"number":      githubv4.Int(number),
	}

	var query pager[ProjectField]
	var queryName string
	switch o.Type {
	case UserOwner:
		variables["login"] = githubv4.String(o.Login)
		query = &userOwnerWithFields{}
		queryName = "UserProjectWithFields"
	case OrgOwner:
		variables["login"] = githubv4.String(o.Login)
		query = &orgOwnerWithFields{}
		queryName = "OrgProjectWithFields"
	case ViewerOwner:
		query = &viewerOwnerWithFields{}
		queryName = "ViewerProjectWithFields"
	}
	err := c.doQueryWithProgressIndicator(queryName, query, variables)
	if err != nil {
		return project, err
	}
	project = query.Project()

	fields, err := paginateAttributes(c, query, variables, queryName, "firstFields", "afterFields", limit, query.Nodes())
	if err != nil {
		return project, err
	}

	project.Fields.Nodes = fields
	return project, nil
}

// viewerLogin is used to query the Login of the viewer.
type viewerLogin struct {
	Viewer struct {
		Login string
		Id    string
	}
}

type viewerLoginOrgs struct {
	Viewer struct {
		Login         string
		ID            string
		Organizations struct {
			PageInfo PageInfo
			Nodes    []struct {
				Login                   string
				ViewerCanCreateProjects bool
				ID                      string
			}
		} `graphql:"organizations(first: 100, after: $after)"`
	}
}

type ownerWithLogin struct {
	Project projectQueryWithoutQueryableItems `graphql:"projectV2(number: $number)"`
	Login   string
}

type ownerWithProjectWithItemQuery struct {
	Project projectQueryWithQueryableItems `graphql:"projectV2(number: $number)"`
}

type ownerWithProjectWithoutItemQuery struct {
	Project projectQueryWithoutQueryableItems `graphql:"projectV2(number: $number)"`
}

// userOwner is used to query the project of a user.
type userOwner struct {
	Owner ownerWithLogin `graphql:"user(login: $login)"`
}

// userOwnerWithItems is used to query the project of a user with its items.
type userOwnerWithItems struct {
	Owner ownerWithProjectWithItemQuery `graphql:"user(login: $login)"`
}

// userOwnerWithItemsNoQuery is used to query the project of a user with its items, without query support.
type userOwnerWithItemsNoQuery struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"user(login: $login)"`
}

// userOwnerWithFields is used to query the project of a user with its fields.
type userOwnerWithFields struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"user(login: $login)"`
}

// orgOwner is used to query the project of an organization.
type orgOwner struct {
	Owner ownerWithLogin `graphql:"organization(login: $login)"`
}

// orgOwnerWithItems is used to query the project of an organization with its items.
type orgOwnerWithItems struct {
	Owner ownerWithProjectWithItemQuery `graphql:"organization(login: $login)"`
}

// orgOwnerWithItemsNoQuery is used to query the project of an organization with its items, without query support.
type orgOwnerWithItemsNoQuery struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"organization(login: $login)"`
}

// orgOwnerWithFields is used to query the project of an organization with its fields.
type orgOwnerWithFields struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"organization(login: $login)"`
}

// viewerOwner is used to query the project of the viewer.
type viewerOwner struct {
	Owner ownerWithLogin `graphql:"viewer"`
}

// viewerOwnerWithItems is used to query the project of the viewer with its items.
type viewerOwnerWithItems struct {
	Owner ownerWithProjectWithItemQuery `graphql:"viewer"`
}

// viewerOwnerWithItemsNoQuery is used to query the project of the viewer with its items, without query support.
type viewerOwnerWithItemsNoQuery struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"viewer"`
}

// viewerOwnerWithFields is used to query the project of the viewer with its fields.
type viewerOwnerWithFields struct {
	Owner ownerWithProjectWithoutItemQuery `graphql:"viewer"`
}

// OwnerType is the type of the owner of a project, which can be either a user or an organization. Viewer is the current user.
type OwnerType string

const UserOwner OwnerType = "USER"
const OrgOwner OwnerType = "ORGANIZATION"
const ViewerOwner OwnerType = "VIEWER"

// ViewerLoginName returns the login name of the viewer.
func (c *Client) ViewerLoginName() (string, error) {
	var query viewerLogin
	err := c.doQueryWithProgressIndicator("Viewer", &query, map[string]interface{}{})
	if err != nil {
		return "", err
	}
	return query.Viewer.Login, nil
}

// OwnerIDAndType returns the ID and OwnerType. The special login "@me" or an empty string queries the current user.
func (c *Client) OwnerIDAndType(login string) (string, OwnerType, error) {
	if login == "@me" || login == "" {
		var query viewerLogin
		err := c.doQueryWithProgressIndicator("ViewerOwner", &query, nil)
		if err != nil {
			return "", "", err
		}
		return query.Viewer.Id, ViewerOwner, nil
	}

	variables := map[string]interface{}{
		"login": githubv4.String(login),
	}
	var query struct {
		User struct {
			Login string
			Id    string
		} `graphql:"user(login: $login)"`
		Organization struct {
			Login string
			Id    string
		} `graphql:"organization(login: $login)"`
	}

	err := c.doQueryWithProgressIndicator("UserOrgOwner", &query, variables)
	if err != nil {
		// Due to the way the queries are structured, we don't know if a login belongs to a user
		// or to an org, even though they are unique. To deal with this, we try both - if neither
		// is found, we return the error.
		var graphErr api.GraphQLError
		if errors.As(err, &graphErr) {
			if graphErr.Match("NOT_FOUND", "user") && graphErr.Match("NOT_FOUND", "organization") {
				return "", "", err
			} else if graphErr.Match("NOT_FOUND", "organization") { // org isn't found must be a user
				return query.User.Id, UserOwner, nil
			} else if graphErr.Match("NOT_FOUND", "user") { // user isn't found must be an org
				return query.Organization.Id, OrgOwner, nil
			}
		}
	}

	return "", "", errors.New("unknown owner type")
}

// issueOrPullRequest is used to query the global id of an issue or pull request by its URL.
type issueOrPullRequest struct {
	Resource struct {
		Typename string `graphql:"__typename"`
		Issue    struct {
			ID string
		} `graphql:"... on Issue"`
		PullRequest struct {
			ID string
		} `graphql:"... on PullRequest"`
	} `graphql:"resource(url: $url)"`
}

// IssueOrPullRequestID returns the ID of the issue or pull request from a URL.
func (c *Client) IssueOrPullRequestID(rawURL string) (string, error) {
	uri, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	variables := map[string]interface{}{
		"url": githubv4.URI{URL: uri},
	}
	var query issueOrPullRequest
	err = c.doQueryWithProgressIndicator("GetIssueOrPullRequest", &query, variables)
	if err != nil {
		return "", err
	}
	if query.Resource.Typename == "Issue" {
		return query.Resource.Issue.ID, nil
	} else if query.Resource.Typename == "PullRequest" {
		return query.Resource.PullRequest.ID, nil
	}
	return "", errors.New("resource not found, please check the URL")
}

// userProjects queries the $first projects of a user.
type userProjects struct {
	Owner struct {
		Projects struct {
			TotalCount int
			PageInfo   PageInfo
			Nodes      []projectQueryWithoutQueryableItems
		} `graphql:"projectsV2(first: $first, after: $after)"`
		Login string
	} `graphql:"user(login: $login)"`
}

// orgProjects queries the $first projects of an organization.
type orgProjects struct {
	Owner struct {
		Projects struct {
			TotalCount int
			PageInfo   PageInfo
			Nodes      []projectQueryWithoutQueryableItems
		} `graphql:"projectsV2(first: $first, after: $after)"`
		Login string
	} `graphql:"organization(login: $login)"`
}

// viewerProjects queries the $first projects of the viewer.
type viewerProjects struct {
	Owner struct {
		Projects struct {
			TotalCount int
			PageInfo   PageInfo
			Nodes      []projectQueryWithoutQueryableItems
		} `graphql:"projectsV2(first: $first, after: $after)"`
		Login string
	} `graphql:"viewer"`
}

type loginTypes struct {
	Login string
	Type  OwnerType
	ID    string
}

// userOrgLogins gets all the logins of the viewer and the organizations the viewer is a member of.
func (c *Client) userOrgLogins() ([]loginTypes, error) {
	l := make([]loginTypes, 0)
	var v viewerLoginOrgs
	variables := map[string]interface{}{
		"after": (*githubv4.String)(nil),
	}

	err := c.doQueryWithProgressIndicator("ViewerLoginAndOrgs", &v, variables)
	if err != nil {
		return l, err
	}

	// add the user
	l = append(l, loginTypes{
		Login: v.Viewer.Login,
		Type:  ViewerOwner,
		ID:    v.Viewer.ID,
	})

	// add orgs where the user can create projects
	for _, org := range v.Viewer.Organizations.Nodes {
		if org.ViewerCanCreateProjects {
			l = append(l, loginTypes{
				Login: org.Login,
				Type:  OrgOwner,
				ID:    org.ID,
			})
		}
	}

	// this seem unlikely, but if there are more org logins, paginate the rest
	if v.Viewer.Organizations.PageInfo.HasNextPage {
		return c.paginateOrgLogins(l, string(v.Viewer.Organizations.PageInfo.EndCursor))
	}

	return l, nil
}

// paginateOrgLogins after cursor and append them to the list of logins.
func (c *Client) paginateOrgLogins(l []loginTypes, cursor string) ([]loginTypes, error) {
	var v viewerLoginOrgs
	variables := map[string]interface{}{
		"after": githubv4.String(cursor),
	}

	err := c.doQueryWithProgressIndicator("ViewerLoginAndOrgs", &v, variables)
	if err != nil {
		return l, err
	}

	for _, org := range v.Viewer.Organizations.Nodes {
		if org.ViewerCanCreateProjects {
			l = append(l, loginTypes{
				Login: org.Login,
				Type:  OrgOwner,
				ID:    org.ID,
			})
		}
	}

	if v.Viewer.Organizations.PageInfo.HasNextPage {
		return c.paginateOrgLogins(l, string(v.Viewer.Organizations.PageInfo.EndCursor))
	}

	return l, nil
}

type Owner struct {
	Login string
	Type  OwnerType
	ID    string
}

// NewOwner creates a project Owner
// If canPrompt is false, login is required as we cannot prompt for it.
// If login is not empty, it is used to lookup the project owner.
// If login is empty, interactive mode is used to select an owner.
// from the current viewer and their organizations
func (c *Client) NewOwner(canPrompt bool, login string) (*Owner, error) {
	if login != "" {
		id, ownerType, err := c.OwnerIDAndType(login)
		if err != nil {
			return nil, err
		}

		return &Owner{
			Login: login,
			Type:  ownerType,
			ID:    id,
		}, nil
	}

	if !canPrompt {
		return nil, fmt.Errorf("owner is required when not running interactively")
	}

	logins, err := c.userOrgLogins()
	if err != nil {
		return nil, err
	}

	options := make([]string, 0, len(logins))
	for _, l := range logins {
		options = append(options, l.Login)
	}

	answerIndex, err := c.prompter.Select("Which owner would you like to use?", "", options)
	if err != nil {
		return nil, err
	}

	l := logins[answerIndex]
	return &Owner{
		Login: l.Login,
		Type:  l.Type,
		ID:    l.ID,
	}, nil
}

// NewProject creates a project based on the owner and project number
// if canPrompt is false, number is required as we cannot prompt for it
// if number is 0 it will prompt the user to select a project interactively
// otherwise it will make a request to get the project by number
// set `fields“ to true to get the project's field data
func (c *Client) NewProject(canPrompt bool, o *Owner, number int32, fields bool) (*Project, error) {
	if number != 0 {
		variables := map[string]interface{}{
			"number":      githubv4.Int(number),
			"firstItems":  githubv4.Int(0),
			"afterItems":  (*githubv4.String)(nil),
			"firstFields": githubv4.Int(0),
			"afterFields": (*githubv4.String)(nil),
		}

		if fields {
			variables["firstFields"] = githubv4.Int(LimitMax)
		}
		if o.Type == UserOwner {
			var query userOwner
			variables["login"] = githubv4.String(o.Login)
			err := c.doQueryWithProgressIndicator("UserProject", &query, variables)
			return newProjectFromQueryWithoutItemsQuery(query.Owner.Project), err
		} else if o.Type == OrgOwner {
			variables["login"] = githubv4.String(o.Login)
			var query orgOwner
			err := c.doQueryWithProgressIndicator("OrgProject", &query, variables)
			return newProjectFromQueryWithoutItemsQuery(query.Owner.Project), err
		} else if o.Type == ViewerOwner {
			var query viewerOwner
			err := c.doQueryWithProgressIndicator("ViewerProject", &query, variables)
			return newProjectFromQueryWithoutItemsQuery(query.Owner.Project), err
		}
		return nil, errors.New("unknown owner type")
	}

	if !canPrompt {
		return nil, fmt.Errorf("project number is required when not running interactively")
	}

	projects, err := c.Projects(o.Login, o.Type, 0, fields)
	if err != nil {
		return nil, err
	}

	if len(projects.Nodes) == 0 {
		return nil, fmt.Errorf("no projects found for %s", o.Login)
	}

	options := make([]string, 0, len(projects.Nodes))
	for _, p := range projects.Nodes {
		title := fmt.Sprintf("%s (#%d)", p.Title, p.Number)
		options = append(options, title)
	}

	answerIndex, err := c.prompter.Select("Which project would you like to use?", "", options)
	if err != nil {
		return nil, err
	}

	return &projects.Nodes[answerIndex], nil
}

// Projects returns all the projects for an Owner. If the OwnerType is VIEWER, no login is required.
// If limit is 0, the default limit is used.
func (c *Client) Projects(login string, t OwnerType, limit int, fields bool) (Projects, error) {
	projects := Projects{
		Nodes: make([]Project, 0),
	}
	cursor := (*githubv4.String)(nil)
	hasNextPage := false

	if limit == 0 {
		limit = LimitDefault
	}

	// set first to the min of limit and LimitMax
	first := LimitMax
	if limit < first {
		first = limit
	}

	variables := map[string]interface{}{
		"first":       githubv4.Int(first),
		"after":       cursor,
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	}

	if fields {
		variables["firstFields"] = githubv4.Int(LimitMax)
	}

	if t != ViewerOwner {
		variables["login"] = githubv4.String(login)
	}
	// loop until we get all the projects
	for {
		// the code below is very repetitive, the only real difference being the type of the query
		// and the query variables. I couldn't figure out a way to make this cleaner that was worth
		// the cost.
		if t == UserOwner {
			var query userProjects
			if err := c.doQueryWithProgressIndicator("UserProjects", &query, variables); err != nil {
				return projects, err
			}
			for _, p := range query.Owner.Projects.Nodes {
				projects.Nodes = append(projects.Nodes, *newProjectFromQueryWithoutItemsQuery(p))
			}
			hasNextPage = query.Owner.Projects.PageInfo.HasNextPage
			cursor = &query.Owner.Projects.PageInfo.EndCursor
			projects.TotalCount = query.Owner.Projects.TotalCount
		} else if t == OrgOwner {
			var query orgProjects
			if err := c.doQueryWithProgressIndicator("OrgProjects", &query, variables); err != nil {
				return projects, err
			}
			for _, p := range query.Owner.Projects.Nodes {
				projects.Nodes = append(projects.Nodes, *newProjectFromQueryWithoutItemsQuery(p))
			}
			hasNextPage = query.Owner.Projects.PageInfo.HasNextPage
			cursor = &query.Owner.Projects.PageInfo.EndCursor
			projects.TotalCount = query.Owner.Projects.TotalCount
		} else if t == ViewerOwner {
			var query viewerProjects
			if err := c.doQueryWithProgressIndicator("ViewerProjects", &query, variables); err != nil {
				return projects, err
			}
			for _, p := range query.Owner.Projects.Nodes {
				projects.Nodes = append(projects.Nodes, *newProjectFromQueryWithoutItemsQuery(p))
			}
			hasNextPage = query.Owner.Projects.PageInfo.HasNextPage
			cursor = &query.Owner.Projects.PageInfo.EndCursor
			projects.TotalCount = query.Owner.Projects.TotalCount
		}

		if !hasNextPage || len(projects.Nodes) >= limit {
			return projects, nil
		}

		if len(projects.Nodes)+LimitMax > limit {
			first := limit - len(projects.Nodes)
			variables["first"] = githubv4.Int(first)
		}
		variables["after"] = cursor
	}
}

type linkProjectToRepoMutation struct {
	LinkProjectV2ToRepository struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"linkProjectV2ToRepository(input:$input)"`
}

type linkProjectToTeamMutation struct {
	LinkProjectV2ToTeam struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"linkProjectV2ToTeam(input:$input)"`
}

type unlinkProjectFromRepoMutation struct {
	UnlinkProjectV2FromRepository struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"unlinkProjectV2FromRepository(input:$input)"`
}

type unlinkProjectFromTeamMutation struct {
	UnlinkProjectV2FromTeam struct {
		ClientMutationId string `graphql:"clientMutationId"`
	} `graphql:"unlinkProjectV2FromTeam(input:$input)"`
}

// LinkProjectToRepository links a project to a repository.
func (c *Client) LinkProjectToRepository(projectID string, repoID string) error {
	var mutation linkProjectToRepoMutation
	variables := map[string]interface{}{
		"input": githubv4.LinkProjectV2ToRepositoryInput{
			ProjectID:    githubv4.String(projectID),
			RepositoryID: githubv4.ID(repoID),
		},
	}

	return c.Mutate("LinkProjectV2ToRepository", &mutation, variables)
}

// LinkProjectToTeam links a project to a team.
func (c *Client) LinkProjectToTeam(projectID string, teamID string) error {
	var mutation linkProjectToTeamMutation
	variables := map[string]interface{}{
		"input": githubv4.LinkProjectV2ToTeamInput{
			ProjectID: githubv4.String(projectID),
			TeamID:    githubv4.ID(teamID),
		},
	}

	return c.Mutate("LinkProjectV2ToTeam", &mutation, variables)
}

// UnlinkProjectFromRepository unlinks a project from a repository.
func (c *Client) UnlinkProjectFromRepository(projectID string, repoID string) error {
	var mutation unlinkProjectFromRepoMutation
	variables := map[string]interface{}{
		"input": githubv4.UnlinkProjectV2FromRepositoryInput{
			ProjectID:    githubv4.String(projectID),
			RepositoryID: githubv4.ID(repoID),
		},
	}

	return c.Mutate("UnlinkProjectV2FromRepository", &mutation, variables)
}

// UnlinkProjectFromTeam unlinks a project from a team.
func (c *Client) UnlinkProjectFromTeam(projectID string, teamID string) error {
	var mutation unlinkProjectFromTeamMutation
	variables := map[string]interface{}{
		"input": githubv4.UnlinkProjectV2FromTeamInput{
			ProjectID: githubv4.String(projectID),
			TeamID:    githubv4.ID(teamID),
		},
	}

	return c.Mutate("UnlinkProjectV2FromTeam", &mutation, variables)
}

func handleError(err error) error {
	var gqlErr api.GraphQLError
	if errors.As(err, &gqlErr) {
		if scopeErr := api.GenerateScopeErrorForGQL(gqlErr.GraphQLError); scopeErr != nil {
			return scopeErr
		}
	}
	return err
}

func projectFieldValueData(v FieldValueNodes) interface{} {
	switch v.Type {
	case "ProjectV2ItemFieldDateValue":
		return v.ProjectV2ItemFieldDateValue.Date
	case "ProjectV2ItemFieldIterationValue":
		return map[string]interface{}{
			"title":       v.ProjectV2ItemFieldIterationValue.Title,
			"startDate":   v.ProjectV2ItemFieldIterationValue.StartDate,
			"duration":    v.ProjectV2ItemFieldIterationValue.Duration,
			"iterationId": v.ProjectV2ItemFieldIterationValue.IterationId,
		}
	case "ProjectV2ItemFieldNumberValue":
		return v.ProjectV2ItemFieldNumberValue.Number
	case "ProjectV2ItemFieldSingleSelectValue":
		return v.ProjectV2ItemFieldSingleSelectValue.Name
	case "ProjectV2ItemFieldTextValue":
		return v.ProjectV2ItemFieldTextValue.Text
	case "ProjectV2ItemFieldMilestoneValue":
		return map[string]interface{}{
			"title":       v.ProjectV2ItemFieldMilestoneValue.Milestone.Title,
			"description": v.ProjectV2ItemFieldMilestoneValue.Milestone.Description,
			"dueOn":       v.ProjectV2ItemFieldMilestoneValue.Milestone.DueOn,
		}
	case "ProjectV2ItemFieldLabelValue":
		names := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldLabelValue.Labels.Nodes {
			names = append(names, p.Name)
		}
		return names
	case "ProjectV2ItemFieldPullRequestValue":
		urls := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldPullRequestValue.PullRequests.Nodes {
			urls = append(urls, p.Url)
		}
		return urls
	case "ProjectV2ItemFieldRepositoryValue":
		return v.ProjectV2ItemFieldRepositoryValue.Repository.Url
	case "ProjectV2ItemFieldUserValue":
		logins := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldUserValue.Users.Nodes {
			logins = append(logins, p.Login)
		}
		return logins
	case "ProjectV2ItemFieldReviewerValue":
		names := make([]string, 0)
		for _, p := range v.ProjectV2ItemFieldReviewerValue.Reviewers.Nodes {
			if p.Type == "Team" {
				names = append(names, p.Team.Name)
			} else if p.Type == "User" {
				names = append(names, p.User.Login)
			}
		}
		return names

	}

	return nil
}

// serialize creates a map from field to field values
func serializeProjectWithItems(project *Project) []map[string]interface{} {
	fields := make(map[string]string)

	// make a map of fields by ID
	for _, f := range project.Fields.Nodes {
		fields[f.ID()] = camelCase(f.Name())
	}
	itemsSlice := make([]map[string]interface{}, 0)

	// for each value, look up the name by ID
	// and set the value to the field value
	for _, i := range project.Items.Nodes {
		o := make(map[string]interface{})
		o["id"] = i.Id
		if projectItem := i.DetailedItem(); projectItem != nil {
			o["content"] = projectItem.ExportData(nil)
		} else {
			o["content"] = nil
		}
		for _, v := range i.FieldValues.Nodes {
			id := v.ID()
			value := projectFieldValueData(v)

			o[fields[id]] = value
		}
		itemsSlice = append(itemsSlice, o)
	}
	return itemsSlice
}

// camelCase converts a string to camelCase, which is useful for turning Go field names to JSON keys.
func camelCase(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) == 1 {
		return strings.ToLower(s)
	}
	return strings.ToLower(s[0:1]) + s[1:]
}

type exportable interface {
	ExportData([]string) map[string]interface{}
}
