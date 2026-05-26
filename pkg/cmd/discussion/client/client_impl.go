package client

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// maxPageSize is the maximum number of items per page allowed by the GitHub GraphQL API.
const maxPageSize = 100

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

// rawActorNode is a JSON-compatible actor type for use with c.gql.GraphQL() (raw
// query strings). Unlike actorNode, it does not use shurcooL graphql struct tags
// and works with standard json.Unmarshal. GitHub flattens inline fragment fields
// (... on User { id name }) to the top level of the actor object in JSON responses.
type rawActorNode struct {
	TypeName string `json:"__typename"`
	Login    string
	ID       string
	Name     string
}

func mapActorFromRawNode(n rawActorNode) DiscussionActor {
	a := DiscussionActor{Login: n.Login}
	switch n.TypeName {
	case "User":
		a.ID = n.ID
		a.Name = n.Name
	case "Bot":
		a.ID = n.ID
	}
	return a
}

// updateDiscussionDiscNode is the JSON response shape for the discussion field
// inside an updateDiscussion mutation response.
type updateDiscussionDiscNode struct {
	ID          string
	Number      int
	Title       string
	Body        string
	URL         string
	Closed      bool
	StateReason string
	Author      rawActorNode
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
	}
	IsAnswered     bool
	AnswerChosenAt time.Time
	AnswerChosenBy *rawActorNode
	ReactionGroups []struct {
		Content string
		Users   struct {
			TotalCount int
		}
	}
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  time.Time
	Locked    bool
	Comments  struct {
		TotalCount int
	}
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

	variables := map[string]interface{}{
		"owner":      githubv4.String(repo.RepoOwner()),
		"name":       githubv4.String(repo.RepoName()),
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
		variables["first"] = githubv4.Int(min(remaining, maxPageSize))
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

	variables := map[string]interface{}{
		"query": githubv4.String(searchQuery),
		"after": (*githubv4.String)(nil),
	}
	if after != "" {
		variables["after"] = githubv4.String(after)
	}

	var result DiscussionListResult
	remaining := limit

	for {
		variables["first"] = githubv4.Int(min(remaining, maxPageSize))
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
			Discussion            struct {
				discussionListNode
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

	err := c.gql.Query(repo.RepoHost(), "DiscussionMinimal", &query, variables)
	if err != nil {
		return nil, err
	}
	if !query.Repository.HasDiscussionsEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has discussions disabled", repo.RepoOwner(), repo.RepoName())
	}

	d := mapDiscussionFromListNode(query.Repository.Discussion.discussionListNode)
	d.Comments = DiscussionCommentList{TotalCount: query.Repository.Discussion.Comments.TotalCount}

	for _, rg := range query.Repository.Discussion.ReactionGroups {
		d.ReactionGroups = append(d.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}

	return &d, nil
}

// discussionReplyNode is the GraphQL response shape for a reply to a discussion comment.
type discussionReplyNode struct {
	ID             string
	URL            string `graphql:"url"`
	Author         actorNode
	Body           string
	CreatedAt      time.Time
	IsAnswer       bool
	UpvoteCount    int
	ReactionGroups []struct {
		Content string
		Users   struct {
			TotalCount int
		}
	}
}

// mapReplyFromNode converts a discussionReplyNode into the domain DiscussionComment type.
func mapReplyFromNode(n discussionReplyNode) DiscussionComment {
	rc := DiscussionComment{
		ID:          n.ID,
		URL:         n.URL,
		Author:      mapActorFromListNode(n.Author),
		Body:        n.Body,
		CreatedAt:   n.CreatedAt,
		IsAnswer:    n.IsAnswer,
		UpvoteCount: n.UpvoteCount,
	}
	for _, rg := range n.ReactionGroups {
		rc.ReactionGroups = append(rc.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}
	return rc
}

// discussionCommentNode is the GraphQL response shape for a discussion comment
// including nested replies.
type discussionCommentNode struct {
	ID             string
	URL            string `graphql:"url"`
	Author         actorNode
	Body           string
	CreatedAt      time.Time
	IsAnswer       bool
	UpvoteCount    int
	ReactionGroups []struct {
		Content string
		Users   struct {
			TotalCount int
		}
	}
	Replies struct {
		TotalCount int
		Nodes      []discussionReplyNode
	} `graphql:"replies(last: 4)"`
}

// mapCommentFromNode converts a discussionCommentNode into the domain DiscussionComment type.
func mapCommentFromNode(n discussionCommentNode) DiscussionComment {
	dc := DiscussionComment{
		ID:          n.ID,
		URL:         n.URL,
		Author:      mapActorFromListNode(n.Author),
		Body:        n.Body,
		CreatedAt:   n.CreatedAt,
		IsAnswer:    n.IsAnswer,
		UpvoteCount: n.UpvoteCount,
	}

	for _, rg := range n.ReactionGroups {
		dc.ReactionGroups = append(dc.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}

	replyComments := make([]DiscussionComment, len(n.Replies.Nodes))
	for i, r := range n.Replies.Nodes {
		replyComments[i] = mapReplyFromNode(r)
	}
	dc.Replies = DiscussionCommentList{
		Comments:   replyComments,
		TotalCount: n.Replies.TotalCount,
		Direction:  DiscussionCommentListDirectionBackward,
	}

	return dc
}

func (c *discussionClient) GetWithComments(repo ghrepo.Interface, number int, limit int, after string, newest bool) (*Discussion, error) {
	var query struct {
		Repository struct {
			HasDiscussionsEnabled bool
			Discussion            struct {
				discussionListNode
				Comments struct {
					TotalCount int
					PageInfo   struct {
						EndCursor       string
						HasNextPage     bool
						StartCursor     string
						HasPreviousPage bool
					}
					Nodes []discussionCommentNode
				} `graphql:"comments(first: $first, last: $last, after: $after, before: $before)"`
			} `graphql:"discussion(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(repo.RepoOwner()),
		"name":   githubv4.String(repo.RepoName()),
		"number": githubv4.Int(number),
		"first":  (*githubv4.Int)(nil),
		"last":   (*githubv4.Int)(nil),
		"after":  (*githubv4.String)(nil),
		"before": (*githubv4.String)(nil),
	}

	if newest {
		variables["last"] = githubv4.Int(limit)
		if after != "" {
			variables["before"] = githubv4.String(after)
		}
	} else {
		variables["first"] = githubv4.Int(limit)
		if after != "" {
			variables["after"] = githubv4.String(after)
		}
	}

	err := c.gql.Query(repo.RepoHost(), "DiscussionWithComments", &query, variables)
	if err != nil {
		return nil, err
	}
	if !query.Repository.HasDiscussionsEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has discussions disabled", repo.RepoOwner(), repo.RepoName())
	}

	src := query.Repository.Discussion

	d := mapDiscussionFromListNode(src.discussionListNode)

	for _, rg := range src.ReactionGroups {
		d.ReactionGroups = append(d.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}

	comments := make([]DiscussionComment, len(src.Comments.Nodes))
	for i, c := range src.Comments.Nodes {
		comments[i] = mapCommentFromNode(c)
	}

	// When using "last" (newest order), the API returns items in chronological
	// order. Reverse them so the newest comment appears first.
	if newest {
		slices.Reverse(comments)
	}

	nextCursor := ""
	if newest {
		if src.Comments.PageInfo.HasPreviousPage {
			nextCursor = src.Comments.PageInfo.StartCursor
		}
	} else {
		if src.Comments.PageInfo.HasNextPage {
			nextCursor = src.Comments.PageInfo.EndCursor
		}
	}

	direction := DiscussionCommentListDirectionForward
	if newest {
		direction = DiscussionCommentListDirectionBackward
	}

	d.Comments = DiscussionCommentList{
		Comments:   comments,
		TotalCount: src.Comments.TotalCount,
		Cursor:     after,
		NextCursor: nextCursor,
		Direction:  direction,
	}

	return &d, nil
}

// GetCommentReplies fetches a discussion and a single comment with its
// paginated replies. It uses the top-level node(id:) query for the comment
// because the Discussion type does not expose a comment(id:) field.
func (c *discussionClient) GetCommentReplies(repo ghrepo.Interface, number int, commentID string, limit int, after string, newest bool) (*Discussion, error) {
	var query struct {
		Repository struct {
			HasDiscussionsEnabled bool
			Discussion            struct {
				discussionListNode
			} `graphql:"discussion(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
		Node *struct {
			DiscussionComment struct {
				ID             string
				URL            string `graphql:"url"`
				Author         actorNode
				Body           string
				CreatedAt      time.Time
				IsAnswer       bool
				UpvoteCount    int
				ReactionGroups []struct {
					Content string
					Users   struct {
						TotalCount int
					}
				}
				Replies struct {
					TotalCount int
					PageInfo   struct {
						EndCursor       string
						HasNextPage     bool
						StartCursor     string
						HasPreviousPage bool
					}
					Nodes []discussionReplyNode
				} `graphql:"replies(first: $first, last: $last, after: $after, before: $before)"`
			} `graphql:"... on DiscussionComment"`
		} `graphql:"node(id: $commentID)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"number":    githubv4.Int(number),
		"commentID": githubv4.ID(commentID),
		"first":     (*githubv4.Int)(nil),
		"last":      (*githubv4.Int)(nil),
		"after":     (*githubv4.String)(nil),
		"before":    (*githubv4.String)(nil),
	}

	if newest {
		variables["last"] = githubv4.Int(limit)
		if after != "" {
			variables["before"] = githubv4.String(after)
		}
	} else {
		variables["first"] = githubv4.Int(limit)
		if after != "" {
			variables["after"] = githubv4.String(after)
		}
	}

	err := c.gql.Query(repo.RepoHost(), "DiscussionCommentReplies", &query, variables)
	if err != nil {
		return nil, err
	}
	if !query.Repository.HasDiscussionsEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has discussions disabled", repo.RepoOwner(), repo.RepoName())
	}

	// The query above should already error for an invalid node ID, but guard against nil.
	if query.Node == nil {
		return nil, fmt.Errorf("comment %s not found", commentID)
	}

	src := query.Node.DiscussionComment
	if src.ID == "" {
		return nil, fmt.Errorf("node %s is not a discussion comment", commentID)
	}

	d := mapDiscussionFromListNode(query.Repository.Discussion.discussionListNode)

	for _, rg := range query.Repository.Discussion.ReactionGroups {
		d.ReactionGroups = append(d.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}

	dc := DiscussionComment{
		ID:          src.ID,
		URL:         src.URL,
		Author:      mapActorFromListNode(src.Author),
		Body:        src.Body,
		CreatedAt:   src.CreatedAt,
		IsAnswer:    src.IsAnswer,
		UpvoteCount: src.UpvoteCount,
	}

	for _, rg := range src.ReactionGroups {
		dc.ReactionGroups = append(dc.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}

	replies := make([]DiscussionComment, len(src.Replies.Nodes))
	for i, r := range src.Replies.Nodes {
		replies[i] = mapReplyFromNode(r)
	}

	// When using "last" (newest order), the API returns items in chronological
	// order. Reverse them so the newest reply appears first.
	if newest {
		slices.Reverse(replies)
	}

	nextCursor := ""
	if newest {
		if src.Replies.PageInfo.HasPreviousPage {
			nextCursor = src.Replies.PageInfo.StartCursor
		}
	} else {
		if src.Replies.PageInfo.HasNextPage {
			nextCursor = src.Replies.PageInfo.EndCursor
		}
	}

	direction := DiscussionCommentListDirectionForward
	if newest {
		direction = DiscussionCommentListDirectionBackward
	}

	dc.Replies = DiscussionCommentList{
		Comments:   replies,
		TotalCount: src.Replies.TotalCount,
		Cursor:     after,
		NextCursor: nextCursor,
		Direction:  direction,
	}

	d.Comments = DiscussionCommentList{
		Comments:   []DiscussionComment{dc},
		TotalCount: 1,
	}

	return &d, nil
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

// repositoryMeta holds the node ID and feature flags fetched for a repository.
type repositoryMeta struct {
	ID                    string
	HasDiscussionsEnabled bool
}

// getRepositoryMeta fetches the node ID and discussion-enabled flag for a repository.
func (c *discussionClient) getRepositoryMeta(repo ghrepo.Interface) (*repositoryMeta, error) {
	var query struct {
		Repository struct {
			ID                    string
			HasDiscussionsEnabled bool
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	if err := c.gql.Query(repo.RepoHost(), "RepositoryMeta", &query, variables); err != nil {
		return nil, err
	}

	return &repositoryMeta{
		ID:                    query.Repository.ID,
		HasDiscussionsEnabled: query.Repository.HasDiscussionsEnabled,
	}, nil
}

// resolveLabels fetches all labels for a repository and matches the requested names
// case-insensitively. Returns an error if any requested label name is not found.
func (c *discussionClient) resolveLabels(repo ghrepo.Interface, labelNames []string) ([]DiscussionLabel, error) {
	if len(labelNames) == 0 {
		return nil, nil
	}

	var query struct {
		Repository struct {
			Labels struct {
				Nodes []struct {
					ID    string
					Name  string
					Color string
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"labels(first: 100, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"endCursor": (*githubv4.String)(nil),
	}

	wanted := make(map[string]bool, len(labelNames))
	for _, n := range labelNames {
		wanted[strings.ToLower(n)] = true
	}

	found := make(map[string]DiscussionLabel, len(labelNames))
	for {
		if err := c.gql.Query(repo.RepoHost(), "RepositoryLabels", &query, variables); err != nil {
			return nil, err
		}
		for _, n := range query.Repository.Labels.Nodes {
			if wanted[strings.ToLower(n.Name)] {
				found[strings.ToLower(n.Name)] = DiscussionLabel{ID: n.ID, Name: n.Name, Color: n.Color}
			}
		}
		if len(found) == len(wanted) {
			break
		}
		if !query.Repository.Labels.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.Labels.PageInfo.EndCursor)
	}

	if len(found) != len(wanted) {
		var missing []string
		for _, name := range labelNames {
			if _, ok := found[strings.ToLower(name)]; !ok {
				missing = append(missing, name)
			}
		}
		return nil, fmt.Errorf("labels not found: %s", strings.Join(missing, ", "))
	}

	result := make([]DiscussionLabel, 0, len(labelNames))
	for _, name := range labelNames {
		result = append(result, found[strings.ToLower(name)])
	}
	return result, nil
}

// editDiscussionLabels adds and removes labels on a discussion. Removals are
// applied before additions. Either slice may be nil or empty to skip that step.
func (c *discussionClient) editDiscussionLabels(repo ghrepo.Interface, discussionID string, addIDs, removeIDs []string) error {
	if len(removeIDs) > 0 {
		ids := make([]githubv4.ID, len(removeIDs))
		for i, id := range removeIDs {
			ids[i] = githubv4.ID(id)
		}

		var mutation struct {
			RemoveLabelsFromLabelable struct {
				Typename string `graphql:"__typename"`
			} `graphql:"removeLabelsFromLabelable(input: $input)"`
		}

		variables := map[string]interface{}{
			"input": githubv4.RemoveLabelsFromLabelableInput{
				LabelableID: githubv4.ID(discussionID),
				LabelIDs:    ids,
			},
		}

		if err := c.gql.Mutate(repo.RepoHost(), "RemoveLabelsFromDiscussion", &mutation, variables); err != nil {
			return err
		}
	}

	if len(addIDs) > 0 {
		ids := make([]githubv4.ID, len(addIDs))
		for i, id := range addIDs {
			ids[i] = githubv4.ID(id)
		}

		var mutation struct {
			AddLabelsToLabelable struct {
				Typename string `graphql:"__typename"`
			} `graphql:"addLabelsToLabelable(input: $input)"`
		}

		variables := map[string]interface{}{
			"input": githubv4.AddLabelsToLabelableInput{
				LabelableID: githubv4.ID(discussionID),
				LabelIDs:    ids,
			},
		}

		if err := c.gql.Mutate(repo.RepoHost(), "AddLabelsToDiscussion", &mutation, variables); err != nil {
			return err
		}
	}

	return nil
}

func (c *discussionClient) Create(repo ghrepo.Interface, input CreateDiscussionInput) (*Discussion, error) {
	meta, err := c.getRepositoryMeta(repo)
	if err != nil {
		return nil, err
	}
	if !meta.HasDiscussionsEnabled {
		return nil, fmt.Errorf("the '%s/%s' repository has discussions disabled", repo.RepoOwner(), repo.RepoName())
	}

	// Resolve labels before creating the discussion so that an unknown label
	// name aborts without leaving a half-created discussion behind.
	var resolvedLabels []DiscussionLabel
	if len(input.Labels) > 0 {
		resolvedLabels, err = c.resolveLabels(repo, input.Labels)
		if err != nil {
			return nil, err
		}
	}

	var mutation struct {
		CreateDiscussion struct {
			Discussion struct {
				discussionListNode
			}
		} `graphql:"createDiscussion(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.CreateDiscussionInput{
			RepositoryID: githubv4.ID(meta.ID),
			CategoryID:   githubv4.ID(input.CategoryID),
			Title:        githubv4.String(input.Title),
			Body:         githubv4.String(input.Body),
		},
	}

	if err := c.gql.Mutate(repo.RepoHost(), "CreateDiscussion", &mutation, variables); err != nil {
		return nil, err
	}

	d := mapDiscussionFromListNode(mutation.CreateDiscussion.Discussion.discussionListNode)

	for _, rg := range mutation.CreateDiscussion.Discussion.ReactionGroups {
		d.ReactionGroups = append(d.ReactionGroups, ReactionGroup{
			Content:    rg.Content,
			TotalCount: rg.Users.TotalCount,
		})
	}

	if len(resolvedLabels) > 0 {
		labelIDs := make([]string, len(resolvedLabels))
		for i, l := range resolvedLabels {
			labelIDs[i] = l.ID
		}
		if err := c.editDiscussionLabels(repo, d.ID, labelIDs, nil); err != nil {
			return nil, err
		}
		d.Labels = resolvedLabels
	}

	return &d, nil
}

const updateDiscussionMutation = `
mutation UpdateDiscussion($input: UpdateDiscussionInput!) {
  updateDiscussion(input: $input) {
    discussion {
      id number title body url closed stateReason
      isAnswered answerChosenAt
      author { __typename login ... on User { id name } ... on Bot { id } }
      answerChosenBy { __typename login ... on User { id name } ... on Bot { id } }
      category { id name slug emoji isAnswerable }
      labels(first: 20) { nodes { id name color } }
      reactionGroups { content users { totalCount } }
      createdAt updatedAt closedAt locked
      comments { totalCount }
    }
  }
}`

func (c *discussionClient) Update(repo ghrepo.Interface, input UpdateDiscussionInput) (*Discussion, error) {
	inputMap := map[string]interface{}{
		"discussionId": input.DiscussionID,
	}
	if input.Title != nil {
		inputMap["title"] = *input.Title
	}
	if input.Body != nil {
		inputMap["body"] = *input.Body
	}
	if input.CategoryID != nil {
		inputMap["categoryId"] = *input.CategoryID
	}

	variables := map[string]interface{}{
		"input": inputMap,
	}

	var result struct {
		UpdateDiscussion struct {
			Discussion updateDiscussionDiscNode
		}
	}
	if err := c.gql.GraphQL(repo.RepoHost(), updateDiscussionMutation, variables, &result); err != nil {
		return nil, err
	}

	disc := result.UpdateDiscussion.Discussion
	d := Discussion{
		ID:             disc.ID,
		Number:         disc.Number,
		Title:          disc.Title,
		Body:           disc.Body,
		URL:            disc.URL,
		Closed:         disc.Closed,
		StateReason:    disc.StateReason,
		Author:         mapActorFromRawNode(disc.Author),
		Category:       DiscussionCategory{ID: disc.Category.ID, Name: disc.Category.Name, Slug: disc.Category.Slug, Emoji: disc.Category.Emoji, IsAnswerable: disc.Category.IsAnswerable},
		Answered:       disc.IsAnswered,
		AnswerChosenAt: disc.AnswerChosenAt,
		CreatedAt:      disc.CreatedAt,
		UpdatedAt:      disc.UpdatedAt,
		ClosedAt:       disc.ClosedAt,
		Locked:         disc.Locked,
		Comments:       DiscussionCommentList{TotalCount: disc.Comments.TotalCount},
	}

	if disc.AnswerChosenBy != nil {
		a := mapActorFromRawNode(*disc.AnswerChosenBy)
		d.AnswerChosenBy = &a
	}

	d.Labels = make([]DiscussionLabel, len(disc.Labels.Nodes))
	for i, l := range disc.Labels.Nodes {
		d.Labels[i] = DiscussionLabel{ID: l.ID, Name: l.Name, Color: l.Color}
	}

	d.ReactionGroups = make([]ReactionGroup, 0, len(disc.ReactionGroups))
	for _, rg := range disc.ReactionGroups {
		d.ReactionGroups = append(d.ReactionGroups, ReactionGroup{Content: rg.Content, TotalCount: rg.Users.TotalCount})
	}

	return &d, nil
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
