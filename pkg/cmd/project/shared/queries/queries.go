package queries

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

type ClientOptions struct {
	Timeout time.Duration
}

func NewClient() (*api.GraphQLClient, error) {
	timeout := 15 * time.Second

	apiOpts := api.ClientOptions{
		Timeout: timeout,
		Headers: map[string]string{},
	}

	return api.NewGraphQLClient(apiOpts)
}

const (
	LimitMax = 100 // https://docs.github.com/en/graphql/overview/resource-limitations#node-limit
)

// doQuery wraps calls to client.Query with a spinner
func doQuery(client *api.GraphQLClient, name string, query interface{}, variables map[string]interface{}) error {
	// https://github.com/briandowns/spinner#available-character-sets
	dotStyle := spinner.CharSets[11]
	sp := spinner.New(dotStyle, 120*time.Millisecond, spinner.WithColor("fgCyan"))
	sp.Start()
	err := client.Query(name, query, variables)
	sp.Stop()
	return err
}

// PageInfo is a PageInfo GraphQL object https://docs.github.com/en/graphql/reference/objects#pageinfo.
type PageInfo struct {
	EndCursor   githubv4.String
	HasNextPage bool
}

// Project is a ProjectV2 GraphQL object https://docs.github.com/en/graphql/reference/objects#projectv2.
type Project struct {
	Number           int
	URL              string
	ShortDescription string
	Public           bool
	Closed           bool
	Title            string
	ID               string
	Readme           string
	Items            struct {
		PageInfo   PageInfo
		TotalCount int
		Nodes      []ProjectItem
	} `graphql:"items(first: $firstItems, after: $afterItems)"`
	Fields struct {
		TotalCount int
		Nodes      []ProjectField
		PageInfo   PageInfo
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

func (p Project) OwnerType() string {
	return p.Owner.TypeName
}

func (p Project) OwnerLogin() string {
	if p.OwnerType() == "User" {
		return p.Owner.User.Login
	}
	return p.Owner.Organization.Login
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
		StartDate string
		Duration  int
		Field     ProjectField
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
		Number float32
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

type PullRequest struct {
	Body       string
	Title      string
	Number     int
	URL        string
	Repository struct {
		NameWithOwner string
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

// ProjectItems returns the items of a project. If the OwnerType is VIEWER, no login is required.
func ProjectItems(client *api.GraphQLClient, o *Owner, number int, limit int) (*Project, error) {
	project := &Project{}
	hasLimit := limit != 0
	// the api limits batches to 100. We want to use the maximum batch size unless the user
	// requested a lower limit.
	first := LimitMax
	if hasLimit && limit < first {
		first = limit
	}
	variables := map[string]interface{}{
		"firstItems":  graphql.Int(first),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": graphql.Int(LimitMax),
		"afterFields": (*githubv4.String)(nil),
		"number":      graphql.Int(number),
	}

	var query pager[ProjectItem]
	var queryName string
	switch o.Type {
	case UserOwner:
		variables["login"] = graphql.String(o.Login)
		query = &userOwnerWithItems{} // must be a pointer to work with graphql queries
		queryName = "UserProjectWithItems"
	case OrgOwner:
		variables["login"] = graphql.String(o.Login)
		query = &orgOwnerWithItems{} // must be a pointer to work with graphql queries
		queryName = "OrgProjectWithItems"
	case ViewerOwner:
		query = &viewerOwnerWithItems{} // must be a pointer to work with graphql queries
		queryName = "ViewerProjectWithItems"
	}
	err := doQuery(client, queryName, query, variables)
	if err != nil {
		return project, err
	}
	project = query.Project()

	items, err := paginateAttributes(client, query, variables, queryName, "firstItems", "afterItems", limit, query.Nodes())
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
	return &q.Owner.Project
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
	return &q.Owner.Project
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
	return &q.Owner.Project
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
	return &q.Owner.Project
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
	return &q.Owner.Project
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
	return &q.Owner.Project
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
func paginateAttributes[N projectAttribute](client *api.GraphQLClient, p pager[N], variables map[string]any, queryName string, firstKey string, afterKey string, limit int, nodes []N) ([]N, error) {
	hasNextPage := p.HasNextPage()
	cursor := p.EndCursor()
	hasLimit := limit != 0
	for {
		if !hasNextPage || (hasLimit && len(nodes) >= limit) {
			return nodes, nil
		}

		if hasLimit && len(nodes)+LimitMax > limit {
			first := limit - len(nodes)
			variables[firstKey] = graphql.Int(first)
		}

		// set the cursor to the end of the last page
		variables[afterKey] = (*githubv4.String)(&cursor)
		err := doQuery(client, queryName, p, variables)
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

// ProjectFields returns a project with fields. If the OwnerType is VIEWER, no login is required.
func ProjectFields(client *api.GraphQLClient, o *Owner, number int, limit int) (*Project, error) {
	project := &Project{}
	hasLimit := limit != 0
	// the api limits batches to 100. We want to use the maximum batch size unless the user
	// requested a lower limit.
	first := LimitMax
	if hasLimit && limit < first {
		first = limit
	}
	variables := map[string]interface{}{
		"firstItems":  graphql.Int(LimitMax),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": graphql.Int(first),
		"afterFields": (*githubv4.String)(nil),
		"number":      graphql.Int(number),
	}

	var query pager[ProjectField]
	var queryName string
	switch o.Type {
	case UserOwner:
		variables["login"] = graphql.String(o.Login)
		query = &userOwnerWithFields{} // must be a pointer to work with graphql queries
		queryName = "UserProjectWithFields"
	case OrgOwner:
		variables["login"] = graphql.String(o.Login)
		query = &orgOwnerWithFields{} // must be a pointer to work with graphql queries
		queryName = "OrgProjectWithFields"
	case ViewerOwner:
		query = &viewerOwnerWithFields{} // must be a pointer to work with graphql queries
		queryName = "ViewerProjectWithFields"
	}
	err := doQuery(client, queryName, query, variables)
	if err != nil {
		return project, err
	}
	project = query.Project()

	fields, err := paginateAttributes(client, query, variables, queryName, "firstFields", "afterFields", limit, query.Nodes())
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

// userLogin is used to query the Login of a user.
type userLogin struct {
	User struct {
		Login string
		Id    string
	} `graphql:"user(login: $login)"`
}

// orgLogin is used to query the Login of an organization.
type orgLogin struct {
	Organization struct {
		Login string
		Id    string
	} `graphql:"organization(login: $login)"`
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

// userOwner is used to query the project of a user.
type userOwner struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
		Login   string
	} `graphql:"user(login: $login)"`
}

// userOwnerWithItems is used to query the project of a user with its items.
type userOwnerWithItems struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
	} `graphql:"user(login: $login)"`
}

// userOwnerWithFields is used to query the project of a user with its fields.
type userOwnerWithFields struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
	} `graphql:"user(login: $login)"`
}

// orgOwner is used to query the project of an organization.
type orgOwner struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
		Login   string
	} `graphql:"organization(login: $login)"`
}

// orgOwnerWithItems is used to query the project of an organization with its items.
type orgOwnerWithItems struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
	} `graphql:"organization(login: $login)"`
}

// orgOwnerWithFields is used to query the project of an organization with its fields.
type orgOwnerWithFields struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
	} `graphql:"organization(login: $login)"`
}

// viewerOwner is used to query the project of the viewer.
type viewerOwner struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
		Login   string
	} `graphql:"viewer"`
}

// viewerOwnerWithItems is used to query the project of the viewer with its items.
type viewerOwnerWithItems struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
	} `graphql:"viewer"`
}

// viewerOwnerWithFields is used to query the project of the viewer with its fields.
type viewerOwnerWithFields struct {
	Owner struct {
		Project Project `graphql:"projectV2(number: $number)"`
	} `graphql:"viewer"`
}

// OwnerType is the type of the owner of a project, which can be either a user or an organization. Viewer is the current user.
type OwnerType string

const UserOwner OwnerType = "USER"
const OrgOwner OwnerType = "ORGANIZATION"
const ViewerOwner OwnerType = "VIEWER"

// ViewerLoginName returns the login name of the viewer.
func ViewerLoginName(client *api.GraphQLClient) (string, error) {
	var query viewerLogin
	err := doQuery(client, "Viewer", &query, map[string]interface{}{})
	if err != nil {
		return "", err
	}
	return query.Viewer.Login, nil
}

// OwnerID returns the ID of an OwnerType. If the OwnerType is VIEWER, no login is required.
func OwnerID(client *api.GraphQLClient, login string, t OwnerType) (string, error) {
	variables := map[string]interface{}{
		"login": graphql.String(login),
	}
	if t == UserOwner {
		var query userLogin
		err := doQuery(client, "UserLogin", &query, variables)
		return query.User.Id, err
	} else if t == OrgOwner {
		var query orgLogin
		err := doQuery(client, "OrgLogin", &query, variables)
		return query.Organization.Id, err
	} else if t == ViewerOwner {
		var query viewerLogin
		err := doQuery(client, "ViewerLogin", &query, nil)
		return query.Viewer.Id, err
	}
	return "", errors.New("unknown owner type")
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
func IssueOrPullRequestID(client *api.GraphQLClient, rawURL string) (string, error) {
	uri, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	variables := map[string]interface{}{
		"url": githubv4.URI{URL: uri},
	}
	var query issueOrPullRequest
	err = doQuery(client, "GetIssueOrPullRequest", &query, variables)
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
			Nodes      []Project
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
			Nodes      []Project
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
			Nodes      []Project
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
func userOrgLogins(client *api.GraphQLClient) ([]loginTypes, error) {
	l := make([]loginTypes, 0)
	var v viewerLoginOrgs
	variables := map[string]interface{}{
		"after": (*githubv4.String)(nil),
	}

	err := doQuery(client, "ViewerLoginAndOrgs", &v, variables)
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
		return paginateOrgLogins(client, l, string(v.Viewer.Organizations.PageInfo.EndCursor))
	}

	return l, nil
}

// paginateOrgLogins after cursor and append them to the list of logins.
func paginateOrgLogins(client *api.GraphQLClient, l []loginTypes, cursor string) ([]loginTypes, error) {
	var v viewerLoginOrgs
	variables := map[string]interface{}{
		"after": (graphql.String)(cursor),
	}

	err := doQuery(client, "ViewerLoginAndOrgs", &v, variables)
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
		return paginateOrgLogins(client, l, string(v.Viewer.Organizations.PageInfo.EndCursor))
	}

	return l, nil
}

type Owner struct {
	Login string
	Type  OwnerType
	ID    string
}

// NewOwner creates a project Owner
// When userLogin == "@me", userLogin becomes the current viewer
// If userLogin is not empty, it is used to lookup the user owner
// If orgLogin is not empty, it is used to lookup the org owner
// If both userLogin and orgLogin are empty, interative mode is used to select an owner
// from the current viewer and their organizations
func NewOwner(client *api.GraphQLClient, userLogin, orgLogin string) (*Owner, error) {
	if userLogin == "@me" {
		id, err := OwnerID(client, userLogin, ViewerOwner)
		if err != nil {
			return nil, err
		}

		return &Owner{
			Login: userLogin,
			Type:  ViewerOwner,
			ID:    id,
		}, nil
	} else if userLogin != "" {
		id, err := OwnerID(client, userLogin, UserOwner)
		if err != nil {
			return nil, err
		}

		return &Owner{
			Login: userLogin,
			Type:  UserOwner,
			ID:    id,
		}, nil
	} else if orgLogin != "" {
		id, err := OwnerID(client, orgLogin, OrgOwner)
		if err != nil {
			return nil, err
		}

		return &Owner{
			Login: orgLogin,
			Type:  OrgOwner,
			ID:    id,
		}, nil
	}

	logins, err := userOrgLogins(client)
	if err != nil {
		return nil, err
	}

	options := make([]string, 0, len(logins))
	for _, l := range logins {
		options = append(options, l.Login)
	}

	var q = []*survey.Question{
		{
			Name: "owner",
			Prompt: &survey.Select{
				Message: "Which owner would you like to use?",
				Options: options,
			},
			Validate: survey.Required,
		},
	}

	answerIndex := 0
	err = survey.Ask(q, &answerIndex)
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
// if number is 0 it will prompt the user to select a project interactively
// otherwise it will make a request to get the project by number
// set `fields“ to true to get the project's field data
func NewProject(client *api.GraphQLClient, o *Owner, number int, fields bool) (*Project, error) {
	if number != 0 {
		variables := map[string]interface{}{
			"number":      graphql.Int(number),
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
			err := doQuery(client, "UserProject", &query, variables)
			return &query.Owner.Project, err
		} else if o.Type == OrgOwner {
			variables["login"] = githubv4.String(o.Login)
			var query orgOwner
			err := doQuery(client, "OrgProject", &query, variables)
			return &query.Owner.Project, err
		} else if o.Type == ViewerOwner {
			var query viewerOwner
			err := doQuery(client, "ViewerProject", &query, variables)
			return &query.Owner.Project, err
		}
		return nil, errors.New("unknown owner type")
	}

	projects, _, err := Projects(client, o.Login, o.Type, 0, fields)
	if err != nil {
		return nil, err
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found for %s", o.Login)
	}

	options := make([]string, 0, len(projects))
	for _, p := range projects {
		title := fmt.Sprintf("%s (#%d)", p.Title, p.Number)
		options = append(options, title)
	}

	var q = []*survey.Question{
		{
			Name: "project",
			Prompt: &survey.Select{
				Message: "Which project would you like to use?",
				Options: options,
			},
			Validate: survey.Required,
		},
	}

	answerIndex := 0
	err = survey.Ask(q, &answerIndex)
	if err != nil {
		return nil, err
	}

	return &projects[answerIndex], nil
}

// Projects returns all the projects for an Owner. If the OwnerType is VIEWER, no login is required.
func Projects(client *api.GraphQLClient, login string, t OwnerType, limit int, fields bool) ([]Project, int, error) {
	projects := make([]Project, 0)
	cursor := (*githubv4.String)(nil)
	hasNextPage := false
	hasLimit := limit != 0
	totalCount := 0

	// the api limits batches to 100. We want to use the maximum batch size unless the user
	// requested a lower limit.
	first := LimitMax
	if hasLimit && limit < first {
		first = limit
	}

	variables := map[string]interface{}{
		"first":       graphql.Int(first),
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
		variables["login"] = graphql.String(login)
	}
	// loop until we get all the projects
	for {
		// the code below is very repetitive, the only real difference being the type of the query
		// and the query variables. I couldn't figure out a way to make this cleaner that was worth
		// the cost.
		if t == UserOwner {
			var query userProjects
			if err := doQuery(client, "UserProjects", &query, variables); err != nil {
				return projects, 0, err
			}
			projects = append(projects, query.Owner.Projects.Nodes...)
			hasNextPage = query.Owner.Projects.PageInfo.HasNextPage
			cursor = &query.Owner.Projects.PageInfo.EndCursor
			totalCount = query.Owner.Projects.TotalCount
		} else if t == OrgOwner {
			var query orgProjects
			if err := doQuery(client, "OrgProjects", &query, variables); err != nil {
				return projects, 0, err
			}
			projects = append(projects, query.Owner.Projects.Nodes...)
			hasNextPage = query.Owner.Projects.PageInfo.HasNextPage
			cursor = &query.Owner.Projects.PageInfo.EndCursor
			totalCount = query.Owner.Projects.TotalCount
		} else if t == ViewerOwner {
			var query viewerProjects
			if err := doQuery(client, "ViewerProjects", &query, variables); err != nil {
				return projects, 0, err
			}
			projects = append(projects, query.Owner.Projects.Nodes...)
			hasNextPage = query.Owner.Projects.PageInfo.HasNextPage
			cursor = &query.Owner.Projects.PageInfo.EndCursor
			totalCount = query.Owner.Projects.TotalCount
		}

		if !hasNextPage || (hasLimit && len(projects) >= limit) {
			return projects, totalCount, nil
		}

		if len(projects)+LimitMax > limit {
			first = limit - len(projects)
			variables["first"] = graphql.Int(first)
		}
		variables["after"] = cursor
	}
}
