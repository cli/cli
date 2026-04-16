package client

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type discussionClient struct {
	gql *api.Client
}

// NewDiscussionClient creates a DiscussionClient backed by the given HTTP client.
func NewDiscussionClient(httpClient *http.Client) DiscussionClient {
	return &discussionClient{
		gql: api.NewClientFromHTTP(httpClient),
	}
}

// actorNode is the GraphQL response shape for an Actor union (User or Bot)
// used in discussionListNode fields like Author and AnswerChosenBy.
type actorNode struct {
	TypeName string `graphql:"__typename"`
	Login    string
	User     struct {
		ID   string
		Name string
	} `graphql:"... on User"`
	Bot struct {
		ID string
	} `graphql:"... on Bot"`
}

// mapActorFromListNode converts an actorNode into the domain DiscussionActor type.
func mapActorFromListNode(n actorNode) DiscussionActor {
	a := DiscussionActor{Login: n.Login}
	switch n.TypeName {
	case "User":
		a.ID = n.User.ID
		a.Name = n.User.Name
	case "Bot":
		a.ID = n.Bot.ID
	}
	return a
}

// discussionListNode is the GraphQL response shape for a discussion in
// list and search results. It covers high-level fields only (no comments, or
// other detail-level data that commands like view would need).
type discussionListNode struct {
	ID          string
	Number      int
	Title       string
	Body        string
	URL         string `graphql:"url"`
	Closed      bool
	StateReason string
	Author      actorNode
	Category    struct {
		ID           string
		Name         string
		Slug         string
		Emoji        string
		IsAnswerable bool
	}
	Labels struct {
		Nodes []struct {
			ID    string
			Name  string
			Color string
		}
	} `graphql:"labels(first: 20)"`
	IsAnswered     bool
	AnswerChosenAt time.Time
	AnswerChosenBy *actorNode
	ReactionGroups []struct {
		Content string
		Users   struct {
			TotalCount int
		}
	} `graphql:"reactionGroups"`
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  time.Time
	Locked    bool
}

// mapDiscussionFromListNode converts a discussionListNode into the domain Discussion type.
func mapDiscussionFromListNode(n discussionListNode) Discussion {
	d := Discussion{
		ID:          n.ID,
		Number:      n.Number,
		Title:       n.Title,
		Body:        n.Body,
		URL:         n.URL,
		Closed:      n.Closed,
		StateReason: n.StateReason,
		Author:      mapActorFromListNode(n.Author),
		Category: DiscussionCategory{
			ID:           n.Category.ID,
			Name:         n.Category.Name,
			Slug:         n.Category.Slug,
			Emoji:        n.Category.Emoji,
			IsAnswerable: n.Category.IsAnswerable,
		},
		Answered:       n.IsAnswered,
		AnswerChosenAt: n.AnswerChosenAt,
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
		ClosedAt:       n.ClosedAt,
		Locked:         n.Locked,
	}

	if n.AnswerChosenBy != nil {
		a := mapActorFromListNode(*n.AnswerChosenBy)
		d.AnswerChosenBy = &a
	}

	d.Labels = make([]DiscussionLabel, len(n.Labels.Nodes))
	for i, l := range n.Labels.Nodes {
		d.Labels[i] = DiscussionLabel{ID: l.ID, Name: l.Name, Color: l.Color}
	}

	return d
}

func (c *discussionClient) List(repo ghrepo.Interface, filters ListFilters, after string, limit int) (*DiscussionListResult, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit argument must be positive: %v", limit)
	}

	var query struct {
		Repository struct {
			HasDiscussionsEnabled bool
			Discussions           struct {
				TotalCount int
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
				Nodes []discussionListNode
			} `graphql:"discussions(first: $first, after: $after, orderBy: $orderBy, categoryId: $categoryId, states: $states, answered: $answered)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	orderField := githubv4.DiscussionOrderFieldUpdatedAt
	orderDir := githubv4.OrderDirectionDesc
	if filters.OrderBy != "" {
		switch filters.OrderBy {
		case OrderByCreated:
			orderField = githubv4.DiscussionOrderFieldCreatedAt
		case OrderByUpdated:
			orderField = githubv4.DiscussionOrderFieldUpdatedAt
		default:
			return nil, fmt.Errorf("unknown order-by field: %q", filters.OrderBy)
		}
	}
	if filters.Direction != "" {
		switch filters.Direction {
		case OrderDirectionAsc:
			orderDir = githubv4.OrderDirectionAsc
		case OrderDirectionDesc:
			orderDir = githubv4.OrderDirectionDesc
		default:
			return nil, fmt.Errorf("unknown order direction: %q", filters.Direction)
		}
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"owner":      githubv4.String(repo.RepoOwner()),
		"name":       githubv4.String(repo.RepoName()),
		"first":      githubv4.Int(perPage),
		"after":      (*githubv4.String)(nil),
		"orderBy":    githubv4.DiscussionOrder{Field: orderField, Direction: orderDir},
		"categoryId": (*githubv4.ID)(nil),
		"states":     (*[]githubv4.DiscussionState)(nil),
		"answered":   (*githubv4.Boolean)(nil),
	}

	if after != "" {
		variables["after"] = githubv4.String(after)
	}

	if filters.CategoryID != "" {
		variables["categoryId"] = githubv4.ID(filters.CategoryID)
	}

	if filters.State != nil {
		switch *filters.State {
		case FilterStateOpen:
			variables["states"] = &[]githubv4.DiscussionState{githubv4.DiscussionStateOpen}
		case FilterStateClosed:
			variables["states"] = &[]githubv4.DiscussionState{githubv4.DiscussionStateClosed}
		default:
			return nil, fmt.Errorf("unknown state filter: %q; should be one of %q, %q", *filters.State, FilterStateOpen, FilterStateClosed)
		}
	}

	if filters.Answered != nil {
		variables["answered"] = githubv4.Boolean(*filters.Answered)
	}

	var result DiscussionListResult
	remaining := limit

	for {
		if err := c.gql.Query(repo.RepoHost(), "DiscussionList", &query, variables); err != nil {
			return nil, err
		}

		if !query.Repository.HasDiscussionsEnabled {
			// This would be the same over every iteration, so if we're going to return we will at the first page.
			return nil, fmt.Errorf("the '%s/%s' repository has discussions disabled", repo.RepoOwner(), repo.RepoName())
		}

		result.TotalCount = query.Repository.Discussions.TotalCount
		for _, n := range query.Repository.Discussions.Nodes {
			result.Discussions = append(result.Discussions, mapDiscussionFromListNode(n))
		}

		remaining -= len(query.Repository.Discussions.Nodes)
		if remaining <= 0 || !query.Repository.Discussions.PageInfo.HasNextPage {
			if query.Repository.Discussions.PageInfo.HasNextPage {
				result.NextCursor = query.Repository.Discussions.PageInfo.EndCursor
			}
			break
		}
		variables["after"] = githubv4.String(query.Repository.Discussions.PageInfo.EndCursor)
	}

	return &result, nil
}

func (c *discussionClient) Search(repo ghrepo.Interface, filters SearchFilters, after string, limit int) (*DiscussionListResult, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit argument must be positive: %v", limit)
	}

	var query struct {
		Search struct {
			DiscussionCount int
			PageInfo        struct {
				HasNextPage bool
				EndCursor   string
			}
			Nodes []struct {
				Discussion discussionListNode `graphql:"... on Discussion"`
			}
		} `graphql:"search(query: $query, type: DISCUSSION, first: $first, after: $after)"`
	}

	qualifiers := []string{fmt.Sprintf("repo:%s/%s", repo.RepoOwner(), repo.RepoName())}

	if filters.State != nil {
		switch *filters.State {
		case FilterStateOpen:
			qualifiers = append(qualifiers, "is:open")
		case FilterStateClosed:
			qualifiers = append(qualifiers, "is:closed")
		default:
			return nil, fmt.Errorf("unknown state filter: %q; should be one of %q, %q", *filters.State, FilterStateOpen, FilterStateClosed)
		}
	}

	if filters.Author != "" {
		qualifiers = append(qualifiers, fmt.Sprintf("author:%q", filters.Author))
	}
	for _, l := range filters.Labels {
		qualifiers = append(qualifiers, fmt.Sprintf("label:%q", l))
	}
	if filters.Category != "" {
		qualifiers = append(qualifiers, fmt.Sprintf("category:%q", filters.Category))
	}
	if filters.Answered != nil {
		if *filters.Answered {
			qualifiers = append(qualifiers, "is:answered")
		} else {
			qualifiers = append(qualifiers, "is:unanswered")
		}
	}

	orderField := "updated"
	orderDir := "desc"
	if filters.OrderBy != "" {
		switch filters.OrderBy {
		case OrderByCreated:
			orderField = "created"
		case OrderByUpdated:
			orderField = "updated"
		default:
			return nil, fmt.Errorf("unknown order-by field: %q", filters.OrderBy)
		}
	}
	if filters.Direction != "" {
		switch filters.Direction {
		case OrderDirectionAsc:
			orderDir = "asc"
		case OrderDirectionDesc:
			orderDir = "desc"
		default:
			return nil, fmt.Errorf("unknown order direction: %q", filters.Direction)
		}
	}
	qualifiers = append(qualifiers, fmt.Sprintf("sort:%s-%s", orderField, orderDir))

	searchQuery := strings.Join(qualifiers, " ")
	if filters.Keywords != "" {
		searchQuery += " " + filters.Keywords
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"query": githubv4.String(searchQuery),
		"first": githubv4.Int(perPage),
		"after": (*githubv4.String)(nil),
	}
	if after != "" {
		variables["after"] = githubv4.String(after)
	}

	var result DiscussionListResult
	remaining := limit

	for {
		if err := c.gql.Query(repo.RepoHost(), "DiscussionListSearch", &query, variables); err != nil {
			return nil, err
		}

		result.TotalCount = query.Search.DiscussionCount
		for _, n := range query.Search.Nodes {
			result.Discussions = append(result.Discussions, mapDiscussionFromListNode(n.Discussion))
		}

		remaining -= len(query.Search.Nodes)
		if remaining <= 0 || !query.Search.PageInfo.HasNextPage {
			if query.Search.PageInfo.HasNextPage {
				result.NextCursor = query.Search.PageInfo.EndCursor
			}
			break
		}
		variables["after"] = githubv4.String(query.Search.PageInfo.EndCursor)
	}

	return &result, nil
}

func (c *discussionClient) GetByNumber(repo ghrepo.Interface, number int) (*Discussion, error) {
	var query struct {
		Repository struct {
			HasDiscussionsEnabled bool
			Discussion            *struct {
				discussionListNode
				Body     string
				Comments struct {
					TotalCount int
				}
			} `graphql:"discussion(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(repo.RepoOwner()),
		"name":   githubv4.String(repo.RepoName()),
		"number": githubv4.Int(number),
	}

	err := c.gql.Query(repo.RepoHost(), "DiscussionByNumber", &query, variables)
	if err != nil {
		return nil, err
	}
	if !query.Repository.HasDiscussionsEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has discussions disabled", repo.RepoOwner(), repo.RepoName())
	}
	if query.Repository.Discussion == nil {
		return nil, fmt.Errorf("discussion #%d not found in '%s/%s'", number, repo.RepoOwner(), repo.RepoName())
	}

	d := mapDiscussionFromListNode(query.Repository.Discussion.discussionListNode)
	d.Body = query.Repository.Discussion.Body
	d.Comments = DiscussionCommentList{TotalCount: query.Repository.Discussion.Comments.TotalCount}

	for _, rg := range query.Repository.Discussion.ReactionGroups {
		d.ReactionGroups = append(d.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}

	return &d, nil
}

func (c *discussionClient) GetWithComments(_ ghrepo.Interface, _ int, _ int, _ string) (*Discussion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *discussionClient) ListCategories(repo ghrepo.Interface) ([]DiscussionCategory, error) {
	var query struct {
		Repository struct {
			HasDiscussionsEnabled bool
			DiscussionCategories  struct {
				Nodes []struct {
					ID           string
					Name         string
					Slug         string
					Emoji        string
					IsAnswerable bool
				}
			} `graphql:"discussionCategories(first: 100)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	if err := c.gql.Query(repo.RepoHost(), "DiscussionCategoryList", &query, variables); err != nil {
		return nil, err
	}

	if !query.Repository.HasDiscussionsEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has discussions disabled", repo.RepoOwner(), repo.RepoName())
	}

	categories := make([]DiscussionCategory, len(query.Repository.DiscussionCategories.Nodes))
	for i, n := range query.Repository.DiscussionCategories.Nodes {
		categories[i] = DiscussionCategory{
			ID:           n.ID,
			Name:         n.Name,
			Slug:         n.Slug,
			Emoji:        n.Emoji,
			IsAnswerable: n.IsAnswerable,
		}
	}

	return categories, nil
}

func (c *discussionClient) Create(_ ghrepo.Interface, _ CreateDiscussionInput) (*Discussion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *discussionClient) Update(_ ghrepo.Interface, _ UpdateDiscussionInput) (*Discussion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *discussionClient) Close(_ ghrepo.Interface, _ string, _ CloseReason) (*Discussion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *discussionClient) Reopen(_ ghrepo.Interface, _ string) (*Discussion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *discussionClient) AddComment(_ ghrepo.Interface, _ string, _ string, _ string) (*DiscussionComment, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *discussionClient) Lock(_ ghrepo.Interface, _ string, _ string) error {
	return fmt.Errorf("not implemented")
}

func (c *discussionClient) Unlock(_ ghrepo.Interface, _ string) error {
	return fmt.Errorf("not implemented")
}

func (c *discussionClient) MarkAnswer(_ ghrepo.Interface, _ string) error {
	return fmt.Errorf("not implemented")
}

func (c *discussionClient) UnmarkAnswer(_ ghrepo.Interface, _ string) error {
	return fmt.Errorf("not implemented")
}
