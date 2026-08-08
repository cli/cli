package client

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDiscussionClient(reg *httpmock.Registry) DiscussionClient {
	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	return NewDiscussionClient(httpClient)
}

// minimalNode returns a minimal JSON discussion node with the given id and title.
func minimalNode(id, title string) string {
	return heredoc.Docf(`
		{
			"id": %q,
			"number": 1,
			"title": %q,
			"body": "",
			"url": "",
			"closed": false,
			"stateReason": "",
			"isAnswered": false,
			"answerChosenAt": "0001-01-01T00:00:00Z",
			"author": {
				"__typename": "User",
				"login": "alice"
			},
			"category": {
				"id": "C1",
				"name": "General",
				"slug": "general",
				"emoji": "",
				"isAnswerable": false
			},
			"answerChosenBy": null,
			"labels": {
				"nodes": []
			},
			"reactionGroups": [],
			"createdAt": "2024-01-01T00:00:00Z",
			"updatedAt": "2024-01-01T00:00:00Z",
			"closedAt": "0001-01-01T00:00:00Z",
			"locked": false
		}
	`, id, title)
}

// minimalNodes returns count comma-separated minimal JSON discussion nodes.
func minimalNodes(count int) string {
	nodes := make([]string, count)
	for i := range nodes {
		nodes[i] = minimalNode(fmt.Sprintf("D%d", i+1), fmt.Sprintf("Discussion %d", i+1))
	}
	return strings.Join(nodes, ",")
}

// listResp builds a mock repository.discussions JSON response.
func listResp(hasNext bool, endCursor string, total int, nodes string) string {
	return heredoc.Docf(`
		{
			"data": {
				"repository": {
					"hasDiscussionsEnabled": true,
					"discussions": {
						"totalCount": %d,
						"pageInfo": {
							"hasNextPage": %t,
							"endCursor": %q
						},
						"nodes": [%s]
					}
				}
			}
		}
	`, total, hasNext, endCursor, nodes)
}

// searchResp builds a mock search JSON response.
func searchResp(hasNext bool, endCursor string, count int, nodes string) string {
	return heredoc.Docf(`
		{
			"data": {
				"search": {
					"discussionCount": %d,
					"pageInfo": {
						"hasNextPage": %t,
						"endCursor": %q
					},
					"nodes": [%s]
				}
			}
		}
	`, count, hasNext, endCursor, nodes)
}

func TestList(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	richNode := heredoc.Doc(`
		{
			"id": "D_rich1",
			"number": 42,
			"title": "Rich discussion",
			"body": "body text here",
			"url": "https://github.com/OWNER/REPO/discussions/42",
			"closed": true,
			"stateReason": "RESOLVED",
			"isAnswered": true,
			"answerChosenAt": "2024-06-01T12:00:00Z",
			"author": {
				"__typename": "User",
				"login": "alice",
				"id": "U1",
				"name": "Alice"
			},
			"category": {
				"id": "C1",
				"name": "Q&A",
				"slug": "q-a",
				"emoji": ":question:",
				"isAnswerable": true
			},
			"answerChosenBy": {
				"__typename": "User",
				"login": "bob",
				"id": "U2",
				"name": "Bob"
			},
			"labels": {
				"nodes": [
					{"id": "L1", "name": "bug", "color": "d73a4a"},
					{"id": "L2", "name": "enhancement", "color": "a2eeef"}
				]
			},
			"reactionGroups": [],
			"createdAt": "2024-01-01T00:00:00Z",
			"updatedAt": "2024-06-02T00:00:00Z",
			"closedAt": "2024-06-01T00:00:00Z",
			"locked": true
		}
	`)

	emptyResp := listResp(false, "", 0, "")
	disabledResp := heredoc.Doc(`
		{
			"data": {
				"repository": {
					"hasDiscussionsEnabled": false,
					"discussions": {
						"totalCount": 0,
						"pageInfo": {
							"hasNextPage": false,
							"endCursor": null
						},
						"nodes": []
					}
				}
			}
		}
	`)

	tests := []struct {
		name           string
		filters        ListFilters
		after          string
		limit          int
		httpStubs      func(*testing.T, *httpmock.Registry)
		wantErr        string
		wantTotal      int
		wantLen        int
		wantNextCursor string
		wantCursor     string
		wantTitles     []string
		wantSingleDisc *Discussion
	}{
		{
			name:  "maps all fields",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(listResp(false, "", 1, richNode)),
				)
			},
			wantTotal: 1,
			wantLen:   1,
			wantSingleDisc: &Discussion{
				ID:          "D_rich1",
				Number:      42,
				Title:       "Rich discussion",
				Body:        "body text here",
				URL:         "https://github.com/OWNER/REPO/discussions/42",
				Closed:      true,
				StateReason: "RESOLVED",
				Author: DiscussionActor{
					ID:    "U1",
					Login: "alice",
					Name:  "Alice",
				},
				Category: DiscussionCategory{
					ID:           "C1",
					Name:         "Q&A",
					Slug:         "q-a",
					Emoji:        ":question:",
					IsAnswerable: true,
				},
				Labels: []DiscussionLabel{
					{ID: "L1", Name: "bug", Color: "d73a4a"},
					{ID: "L2", Name: "enhancement", Color: "a2eeef"},
				},
				Answered:       true,
				AnswerChosenAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				AnswerChosenBy: &DiscussionActor{
					ID:    "U2",
					Login: "bob",
					Name:  "Bob",
				},
				Comments:  DiscussionCommentList{},
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC),
				ClosedAt:  time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Locked:    true,
			},
		},
		{
			name:  "empty list",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(emptyResp),
				)
			},
			wantTotal: 0,
			wantLen:   0,
		},
		{
			name:  "discussions disabled",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(disabledResp),
				)
			},
			wantErr: "discussions disabled",
		},
		{
			name:    "limit zero",
			limit:   0,
			wantErr: "limit argument must be positive",
		},
		{
			name:    "invalid orderBy",
			limit:   10,
			filters: ListFilters{OrderBy: "invalid"},
			wantErr: "unknown order-by field",
		},
		{
			name:    "invalid direction",
			limit:   10,
			filters: ListFilters{Direction: "sideways"},
			wantErr: "unknown order direction",
		},
		{
			name:    "invalid state",
			limit:   10,
			filters: ListFilters{State: new("merged")},
			wantErr: "unknown state filter",
		},
		{
			name:  "with after cursor",
			limit: 10,
			after: "someCursor",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Equal(t, "someCursor", vars["after"])
					}),
				)
			},
			wantCursor: "someCursor",
		},
		{
			name:    "open state filter",
			limit:   10,
			filters: ListFilters{State: new(FilterStateOpen)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"OPEN"}, vars["states"])
					}),
				)
			},
		},
		{
			name:    "closed state filter",
			limit:   10,
			filters: ListFilters{State: new(FilterStateClosed)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"CLOSED"}, vars["states"])
					}),
				)
			},
		},
		{
			name:    "answered filter",
			limit:   10,
			filters: ListFilters{Answered: new(true)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Equal(t, true, vars["answered"])
					}),
				)
			},
		},
		{
			name:    "unanswered filter",
			limit:   10,
			filters: ListFilters{Answered: new(false)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Equal(t, false, vars["answered"])
					}),
				)
			},
		},
		{
			name:    "category ID filter",
			limit:   10,
			filters: ListFilters{CategoryID: "CAT123"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Equal(t, "CAT123", vars["categoryId"])
					}),
				)
			},
		},
		{
			name:    "order by created asc",
			limit:   10,
			filters: ListFilters{OrderBy: OrderByCreated, Direction: OrderDirectionAsc},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						orderBy, ok := vars["orderBy"].(map[string]interface{})
						require.True(t, ok, "orderBy should be a map")
						assert.Equal(t, "CREATED_AT", orderBy["field"])
						assert.Equal(t, "ASC", orderBy["direction"])
					}),
				)
			},
		},
		{
			name:    "order by updated desc",
			limit:   10,
			filters: ListFilters{OrderBy: OrderByUpdated, Direction: OrderDirectionDesc},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						orderBy, ok := vars["orderBy"].(map[string]interface{})
						require.True(t, ok, "orderBy should be a map")
						assert.Equal(t, "UPDATED_AT", orderBy["field"])
						assert.Equal(t, "DESC", orderBy["direction"])
					}),
				)
			},
		},
		{
			// Bot actors have no name; ID comes from the Bot.ID field.
			name:  "bot actor",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(listResp(false, "", 1, heredoc.Doc(`
						{
							"id": "D_bot",
							"number": 1,
							"title": "Bot post",
							"body": "",
							"url": "",
							"closed": false,
							"stateReason": "",
							"isAnswered": false,
							"answerChosenAt": "0001-01-01T00:00:00Z",
							"author": {
								"__typename": "Bot",
								"login": "gh-bot",
								"id": "bot-node-id"
							},
							"category": {
								"id": "C1",
								"name": "General",
								"slug": "general",
								"emoji": "",
								"isAnswerable": false
							},
							"answerChosenBy": null,
							"labels": {
								"nodes": []
							},
							"reactionGroups": [],
							"createdAt": "2024-01-01T00:00:00Z",
							"updatedAt": "2024-01-01T00:00:00Z",
							"closedAt": "0001-01-01T00:00:00Z",
							"locked": false
						}
					`))),
				)
			},
			wantLen:   1,
			wantTotal: 1,
			wantSingleDisc: &Discussion{
				ID:        "D_bot",
				Number:    1,
				Title:     "Bot post",
				Author:    DiscussionActor{ID: "bot-node-id", Login: "gh-bot", Name: ""},
				Category:  DiscussionCategory{ID: "C1", Name: "General", Slug: "general"},
				Labels:    []DiscussionLabel{},
				Comments:  DiscussionCommentList{},
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			// When limit > 100, the first page requests 100 and the second page
			// requests the remainder, exercising the per-iteration first variable.
			name:  "limit greater than 100",
			limit: 101,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(listResp(true, "pg2cursor", 101, minimalNodes(100)), func(_ string, vars map[string]interface{}) {
						assert.Equal(t, float64(100), vars["first"])
					}),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.GraphQLQuery(listResp(false, "", 101, minimalNode("D101", "Discussion 101")), func(_ string, vars map[string]interface{}) {
						assert.Equal(t, float64(1), vars["first"])
					}),
				)
			},
			wantLen:   101,
			wantTotal: 101,
		},
		{
			// When the page has more items than requested, NextCursor is set.
			name:  "pagination sets next cursor",
			limit: 1,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(listResp(true, "cursor42", 5, minimalNode("D1", "Discussion 1"))),
				)
			},
			wantLen:        1,
			wantTotal:      5,
			wantNextCursor: "cursor42",
		},
		{
			// Two pages are fetched when limit exceeds the first page's results.
			name:  "pagination fetches multiple pages",
			limit: 2,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(listResp(true, "cursor1", 2, minimalNode("D1", "First"))),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(listResp(false, "", 2, minimalNode("D2", "Second"))),
				)
			},
			wantLen:    2,
			wantTotal:  2,
			wantTitles: []string{"First", "Second"},
		},
		{
			name:  "exact fit does not overfetch",
			limit: 1,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionList\b`),
					httpmock.StringResponse(listResp(false, "", 1, minimalNode("D1", "Only one"))),
				)
			},
			wantLen:    1,
			wantTotal:  1,
			wantTitles: []string{"Only one"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			result, err := c.List(repo, tt.filters, tt.after, tt.limit)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantTotal, result.TotalCount)
			assert.Len(t, result.Discussions, tt.wantLen)
			assert.Equal(t, tt.wantCursor, result.Cursor)
			assert.Equal(t, tt.wantNextCursor, result.NextCursor)

			for i, title := range tt.wantTitles {
				assert.Equal(t, title, result.Discussions[i].Title)
			}

			if tt.wantSingleDisc != nil {
				require.NotEmpty(t, result.Discussions)
				assert.Equal(t, *tt.wantSingleDisc, result.Discussions[0])
			}
		})
	}
}

func TestSearch(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	richNode := heredoc.Doc(`
		{
			"id": "D_rich1",
			"number": 42,
			"title": "Rich search result",
			"body": "body text here",
			"url": "https://github.com/OWNER/REPO/discussions/42",
			"closed": true,
			"stateReason": "RESOLVED",
			"isAnswered": true,
			"answerChosenAt": "2024-06-01T12:00:00Z",
			"author": {
				"__typename": "User",
				"login": "alice",
				"id": "U1",
				"name": "Alice"
			},
			"category": {
				"id": "C1",
				"name": "Q&A",
				"slug": "q-a",
				"emoji": ":question:",
				"isAnswerable": true
			},
			"answerChosenBy": {
				"__typename": "User",
				"login": "bob",
				"id": "U2",
				"name": "Bob"
			},
			"labels": {
				"nodes": [
					{"id": "L1", "name": "bug", "color": "d73a4a"}
				]
			},
			"reactionGroups": [],
			"createdAt": "2024-01-01T00:00:00Z",
			"updatedAt": "2024-06-02T00:00:00Z",
			"closedAt": "2024-06-01T00:00:00Z",
			"locked": true
		}
	`)

	emptyResp := searchResp(false, "", 0, "")

	tests := []struct {
		name           string
		filters        SearchFilters
		after          string
		limit          int
		httpStubs      func(*testing.T, *httpmock.Registry)
		wantErr        string
		wantTotal      int
		wantLen        int
		wantCursor     string
		wantNextCursor string
		wantTitles     []string
		wantSingleDisc *Discussion
	}{
		{
			name:  "maps all fields",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.StringResponse(searchResp(false, "", 1, richNode)),
				)
			},
			wantTotal: 1,
			wantLen:   1,
			wantSingleDisc: &Discussion{
				ID:          "D_rich1",
				Number:      42,
				Title:       "Rich search result",
				Body:        "body text here",
				URL:         "https://github.com/OWNER/REPO/discussions/42",
				Closed:      true,
				StateReason: "RESOLVED",
				Author: DiscussionActor{
					ID:    "U1",
					Login: "alice",
					Name:  "Alice",
				},
				Category: DiscussionCategory{
					ID:           "C1",
					Name:         "Q&A",
					Slug:         "q-a",
					Emoji:        ":question:",
					IsAnswerable: true,
				},
				Labels: []DiscussionLabel{
					{ID: "L1", Name: "bug", Color: "d73a4a"},
				},
				Answered:       true,
				AnswerChosenAt: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
				AnswerChosenBy: &DiscussionActor{
					ID:    "U2",
					Login: "bob",
					Name:  "Bob",
				},
				Comments:  DiscussionCommentList{},
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC),
				ClosedAt:  time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Locked:    true,
			},
		},
		{
			name:    "limit zero",
			limit:   0,
			wantErr: "limit argument must be positive",
		},
		{
			name:    "invalid orderBy",
			limit:   10,
			filters: SearchFilters{OrderBy: "bogus"},
			wantErr: "unknown order-by field",
		},
		{
			name:    "invalid direction",
			limit:   10,
			filters: SearchFilters{Direction: "sideways"},
			wantErr: "unknown order direction",
		},
		{
			name:    "invalid state",
			limit:   10,
			filters: SearchFilters{State: new("merged")},
			wantErr: "unknown state filter",
		},
		{
			name:  "with after cursor",
			limit: 10,
			after: "someCursor",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Equal(t, "someCursor", vars["after"])
					}),
				)
			},
			wantCursor: "someCursor",
		},
		{
			name:    "open state filter",
			limit:   10,
			filters: SearchFilters{State: new(FilterStateOpen)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), "is:open")
					}),
				)
			},
		},
		{
			name:    "closed state filter",
			limit:   10,
			filters: SearchFilters{State: new(FilterStateClosed)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), "is:closed")
					}),
				)
			},
		},
		{
			name:    "answered filter",
			limit:   10,
			filters: SearchFilters{Answered: new(true)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), "is:answered")
					}),
				)
			},
		},
		{
			name:    "unanswered filter",
			limit:   10,
			filters: SearchFilters{Answered: new(false)},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), "is:unanswered")
					}),
				)
			},
		},
		{
			name:    "author filter",
			limit:   10,
			filters: SearchFilters{Author: "alice"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), `author:"alice"`)
					}),
				)
			},
		},
		{
			name:    "labels filter",
			limit:   10,
			filters: SearchFilters{Labels: []string{"bug", "enhancement"}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						q := vars["query"].(string)
						assert.Contains(t, q, `label:"bug"`)
						assert.Contains(t, q, `label:"enhancement"`)
					}),
				)
			},
		},
		{
			name:    "category filter",
			limit:   10,
			filters: SearchFilters{Category: "Q&A"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), `category:"Q&A"`)
					}),
				)
			},
		},
		{
			name:    "keywords filter",
			limit:   10,
			filters: SearchFilters{Keywords: "some keyword"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), "some keyword")
					}),
				)
			},
		},
		{
			name:    "order by created asc",
			limit:   10,
			filters: SearchFilters{OrderBy: OrderByCreated, Direction: OrderDirectionAsc},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), "sort:created-asc")
					}),
				)
			},
		},
		{
			name:    "order by updated desc",
			limit:   10,
			filters: SearchFilters{OrderBy: OrderByUpdated, Direction: OrderDirectionDesc},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(emptyResp, func(_ string, vars map[string]interface{}) {
						assert.Contains(t, vars["query"].(string), "sort:updated-desc")
					}),
				)
			},
		},
		{
			// When limit > 100, the first page requests 100 and the second page
			// requests the remainder, exercising the per-iteration first variable.
			name:  "limit greater than 100",
			limit: 101,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(searchResp(true, "pg2cursor", 101, minimalNodes(100)), func(_ string, vars map[string]interface{}) {
						assert.Equal(t, float64(100), vars["first"])
					}),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.GraphQLQuery(searchResp(false, "", 101, minimalNode("D101", "Discussion 101")), func(_ string, vars map[string]interface{}) {
						assert.Equal(t, float64(1), vars["first"])
					}),
				)
			},
			wantLen:   101,
			wantTotal: 101,
		},
		{
			// When the page has more items than requested, NextCursor is set.
			name:  "pagination sets next cursor",
			limit: 1,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.StringResponse(searchResp(true, "searchCursor42", 5, minimalNode("D1", "Discussion 1"))),
				)
			},
			wantLen:        1,
			wantTotal:      5,
			wantNextCursor: "searchCursor42",
		},
		{
			// Two pages are fetched when limit exceeds the first page's results.
			name:  "pagination fetches multiple pages",
			limit: 2,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.StringResponse(searchResp(true, "searchCursor1", 2, minimalNode("D1", "First"))),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.StringResponse(searchResp(false, "", 2, minimalNode("D2", "Second"))),
				)
			},
			wantLen:    2,
			wantTotal:  2,
			wantTitles: []string{"First", "Second"},
		},
		{
			name:  "exact fit does not overfetch",
			limit: 1,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionListSearch\b`),
					httpmock.StringResponse(searchResp(false, "", 1, minimalNode("D1", "Only one"))),
				)
			},
			wantLen:    1,
			wantTotal:  1,
			wantTitles: []string{"Only one"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			result, err := c.Search(repo, tt.filters, tt.after, tt.limit)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantTotal, result.TotalCount)
			assert.Len(t, result.Discussions, tt.wantLen)
			assert.Equal(t, tt.wantCursor, result.Cursor)
			assert.Equal(t, tt.wantNextCursor, result.NextCursor)

			for i, title := range tt.wantTitles {
				assert.Equal(t, title, result.Discussions[i].Title)
			}

			if tt.wantSingleDisc != nil {
				require.NotEmpty(t, result.Discussions)
				assert.Equal(t, *tt.wantSingleDisc, result.Discussions[0])
			}
		})
	}
}

func TestListCategories(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name      string
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantCats  []DiscussionCategory
	}{
		{
			name: "maps all fields",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCategoryList\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasDiscussionsEnabled":true,
						"discussionCategories":{"nodes":[
							{"id":"C1","name":"General","slug":"general","emoji":":speech_balloon:","isAnswerable":false},
							{"id":"C2","name":"Q&A","slug":"q-a","emoji":":question:","isAnswerable":true}
						]}
					}}}`),
				)
			},
			wantCats: []DiscussionCategory{
				{ID: "C1", Name: "General", Slug: "general", Emoji: ":speech_balloon:", IsAnswerable: false},
				{ID: "C2", Name: "Q&A", Slug: "q-a", Emoji: ":question:", IsAnswerable: true},
			},
		},
		{
			name: "discussions disabled",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCategoryList\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasDiscussionsEnabled":false,
						"discussionCategories":{"nodes":[]}
					}}}`),
				)
			},
			wantErr: "discussions disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			categories, err := c.ListCategories(repo)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Len(t, categories, len(tt.wantCats))
			for i, want := range tt.wantCats {
				assert.Equal(t, want, categories[i])
			}
		})
	}
}

func TestGetByNumber(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name       string
		httpStubs  func(*testing.T, *httpmock.Registry)
		wantErr    string
		assertDisc *Discussion
	}{
		{
			name: "maps all fields",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionMinimal\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 42,
										"title": "Test Discussion",
										"body": "This is a test",
										"url": "https://github.com/OWNER/REPO/discussions/42",
										"closed": true,
										"stateReason": "RESOLVED",
										"isAnswered": true,
										"answerChosenAt": "2025-06-01T12:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "C1", "name": "Q&A", "slug": "q-a", "emoji": ":question:", "isAnswerable": true},
										"answerChosenBy": {"__typename": "User", "login": "bob", "id": "U2", "name": "Bob"},
										"labels": {"nodes": [{"id": "L1", "name": "bug", "color": "d73a4a"}]},
										"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 3}}],
										"createdAt": "2025-01-01T00:00:00Z",
										"updatedAt": "2025-01-02T00:00:00Z",
										"closedAt": "2025-06-01T00:00:00Z",
										"locked": true,
										"comments": {"totalCount": 5}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: &Discussion{
				ID:          "D_1",
				Number:      42,
				Title:       "Test Discussion",
				Body:        "This is a test",
				URL:         "https://github.com/OWNER/REPO/discussions/42",
				Closed:      true,
				StateReason: "RESOLVED",
				Author:      DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:           "C1",
					Name:         "Q&A",
					Slug:         "q-a",
					Emoji:        ":question:",
					IsAnswerable: true,
				},
				Labels:         []DiscussionLabel{{ID: "L1", Name: "bug", Color: "d73a4a"}},
				Answered:       true,
				AnswerChosenAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
				AnswerChosenBy: &DiscussionActor{ID: "U2", Login: "bob", Name: "Bob"},
				ReactionGroups: []ReactionGroup{
					{Content: "THUMBS_UP", TotalCount: 3},
				},
				Comments:  DiscussionCommentList{TotalCount: 5},
				CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				ClosedAt:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				Locked:    true,
			},
		},
		{
			name: "discussions disabled",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", false)),
				)
			},
			wantErr: "has discussions disabled",
		},
		{
			name: "repo not found",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": null
							},
							"errors": [
								{
									"type": "NOT_FOUND",
									"path": ["repository"],
									"message": "Could not resolve to a Repository with the name 'OWNER/REPO'."
								}
							]
						}
					`)),
				)
			},
			wantErr: "Could not resolve to a Repository with the name 'OWNER/REPO'.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			d, err := c.GetByNumber(repo, 42)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, d)
			require.NotNil(t, tt.assertDisc, "assertDisc must be set for non-error cases")
			assert.Equal(t, tt.assertDisc, d)
		})
	}
}

func TestGetWithComments(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name       string
		limit      int
		after      string
		newest     bool
		httpStubs  func(*testing.T, *httpmock.Registry)
		wantErr    string
		assertDisc func(*testing.T, *Discussion)
	}{
		{
			name:   "maps comments with replies",
			limit:  10,
			newest: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionWithComments\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 42,
										"title": "Test Discussion",
										"body": "Discussion body",
										"url": "https://github.com/OWNER/REPO/discussions/42",
										"closed": true,
										"stateReason": "RESOLVED",
										"isAnswered": true,
										"answerChosenAt": "2025-06-01T12:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U_alice", "name": "Alice"},
										"category": {"id": "CAT1", "name": "Q&A", "slug": "q-a", "emoji": ":question:", "isAnswerable": true},
										"answerChosenBy": {"__typename": "User", "login": "bob", "id": "U_bob", "name": "Bob"},
										"labels": {"nodes": [{"id": "L1", "name": "bug", "color": "d73a4a"}]},
										"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 3}}],
										"createdAt": "2025-01-01T00:00:00Z",
										"updatedAt": "2025-01-02T00:00:00Z",
										"closedAt": "2025-06-01T00:00:00Z",
										"locked": true,
										"comments": {
											"totalCount": 1,
											"pageInfo": {"endCursor": "COM_CUR", "hasNextPage": true, "startCursor": "COM_START", "hasPreviousPage": false},
											"nodes": [
												{
													"id": "C1",
													"url": "https://github.com/OWNER/REPO/discussions/42#comment-1",
													"author": {"__typename": "User", "login": "octocat", "id": "U_octocat", "name": "Octocat"},
													"body": "Main comment",
													"createdAt": "2025-03-01T00:00:00Z",
													"isAnswer": true,
													"upvoteCount": 5,
													"reactionGroups": [{"content": "HEART", "users": {"totalCount": 2}}],
													"replies": {
														"totalCount": 1,
														"nodes": [
															{
																"id": "R1",
																"url": "https://github.com/OWNER/REPO/discussions/42#reply-1",
																"author": {"__typename": "User", "login": "hubot", "id": "U_hubot", "name": "Hubot"},
																"body": "Thanks!",
																"createdAt": "2025-04-01T00:00:00Z",
																"isAnswer": false,
																"upvoteCount": 1,
																"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 1}}]
															}
														]
													}
												}
											]
										}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				assert.Equal(t, Discussion{
					ID:          "D_1",
					Number:      42,
					Title:       "Test Discussion",
					Body:        "Discussion body",
					URL:         "https://github.com/OWNER/REPO/discussions/42",
					Closed:      true,
					StateReason: "RESOLVED",
					Author:      DiscussionActor{ID: "U_alice", Login: "alice", Name: "Alice"},
					Category: DiscussionCategory{
						ID:           "CAT1",
						Name:         "Q&A",
						Slug:         "q-a",
						Emoji:        ":question:",
						IsAnswerable: true,
					},
					Labels:         []DiscussionLabel{{ID: "L1", Name: "bug", Color: "d73a4a"}},
					Answered:       true,
					AnswerChosenAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
					AnswerChosenBy: &DiscussionActor{ID: "U_bob", Login: "bob", Name: "Bob"},
					ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 3}},
					CreatedAt:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt:      time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
					ClosedAt:       time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
					Locked:         true,
					Comments: DiscussionCommentList{
						TotalCount: 1,
						NextCursor: "COM_CUR",
						Direction:  DiscussionCommentListDirectionForward,
						Comments: []DiscussionComment{
							{
								ID:             "C1",
								URL:            "https://github.com/OWNER/REPO/discussions/42#comment-1",
								Author:         DiscussionActor{ID: "U_octocat", Login: "octocat", Name: "Octocat"},
								Body:           "Main comment",
								CreatedAt:      time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
								IsAnswer:       true,
								UpvoteCount:    5,
								ReactionGroups: []ReactionGroup{{Content: "HEART", TotalCount: 2}},
								Replies: DiscussionCommentList{
									TotalCount: 1,
									Direction:  DiscussionCommentListDirectionBackward,
									Comments: []DiscussionComment{
										{
											ID:             "R1",
											URL:            "https://github.com/OWNER/REPO/discussions/42#reply-1",
											Author:         DiscussionActor{ID: "U_hubot", Login: "hubot", Name: "Hubot"},
											Body:           "Thanks!",
											CreatedAt:      time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
											UpvoteCount:    1,
											ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 1}},
										},
									},
								},
							},
						},
					},
				}, *d)
			},
		},
		{
			name:   "pagination forward",
			limit:  5,
			after:  "CUR_A",
			newest: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionWithComments\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false,
										"comments": {
											"totalCount": 3,
											"pageInfo": {"endCursor": "CUR_B", "hasNextPage": true, "startCursor": "", "hasPreviousPage": false},
											"nodes": [
												{
													"id": "C1",
													"url": "",
													"author": {"__typename": "User", "login": "alice"},
													"body": "Hello",
													"createdAt": "2025-01-01T00:00:00Z",
													"isAnswer": false,
													"upvoteCount": 0,
													"reactionGroups": [],
													"replies": {"totalCount": 0, "nodes": []}
												}
											]
										}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				comments := d.Comments
				assert.Len(t, comments.Comments, 1)
				assert.Equal(t, 3, comments.TotalCount)
				assert.Equal(t, "CUR_A", comments.Cursor)
				assert.Equal(t, "CUR_B", comments.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionForward, comments.Direction)
			},
		},
		{
			name:   "pagination backward newest",
			limit:  5,
			after:  "CUR_X",
			newest: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionWithComments\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false,
										"comments": {
											"totalCount": 5,
											"pageInfo": {"endCursor": "", "hasNextPage": false, "startCursor": "CUR_Y", "hasPreviousPage": true},
											"nodes": [
												{
													"id": "C1",
													"url": "",
													"author": {"__typename": "User", "login": "alice"},
													"body": "First",
													"createdAt": "2025-01-01T00:00:00Z",
													"isAnswer": false,
													"upvoteCount": 0,
													"reactionGroups": [],
													"replies": {"totalCount": 0, "nodes": []}
												},
												{
													"id": "C2",
													"url": "",
													"author": {"__typename": "User", "login": "bob"},
													"body": "Second",
													"createdAt": "2025-01-02T00:00:00Z",
													"isAnswer": false,
													"upvoteCount": 0,
													"reactionGroups": [],
													"replies": {"totalCount": 0, "nodes": []}
												}
											]
										}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				comments := d.Comments
				assert.Len(t, comments.Comments, 2)
				assert.Equal(t, 5, comments.TotalCount)
				assert.Equal(t, "CUR_X", comments.Cursor)
				assert.Equal(t, "CUR_Y", comments.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionBackward, comments.Direction)
				assert.Equal(t, "C2", comments.Comments[0].ID, "newest mode should reverse comments")
				assert.Equal(t, "C1", comments.Comments[1].ID)
			},
		},
		{
			name:   "no more pages",
			limit:  10,
			newest: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionWithComments\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false,
										"comments": {
											"totalCount": 1,
											"pageInfo": {"endCursor": "", "hasNextPage": false, "startCursor": "", "hasPreviousPage": false},
											"nodes": [
												{
													"id": "C1",
													"url": "",
													"author": {"__typename": "User", "login": "alice"},
													"body": "Only one",
													"createdAt": "2025-01-01T00:00:00Z",
													"isAnswer": false,
													"upvoteCount": 0,
													"reactionGroups": [],
													"replies": {"totalCount": 0, "nodes": []}
												}
											]
										}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				comments := d.Comments
				assert.Len(t, comments.Comments, 1)
				assert.Equal(t, 1, comments.TotalCount)
				assert.Equal(t, "", comments.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionForward, comments.Direction)
			},
		},
		{
			name:   "discussions disabled",
			limit:  10,
			newest: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", false)),
				)
			},
			wantErr: "has discussions disabled",
		},
		{
			name:   "repo not found",
			limit:  10,
			newest: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": null
							},
							"errors": [
								{
									"type": "NOT_FOUND",
									"path": ["repository"],
									"message": "Could not resolve to a Repository with the name 'OWNER/REPO'."
								}
							]
						}
					`)),
				)
			},
			wantErr: "Could not resolve to a Repository with the name 'OWNER/REPO'.",
		},
		{
			name:   "empty comments",
			limit:  10,
			newest: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionWithComments\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false,
										"comments": {
											"totalCount": 0,
											"pageInfo": {"endCursor": null, "hasNextPage": false, "startCursor": null, "hasPreviousPage": false},
											"nodes": []
										}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				comments := d.Comments
				assert.Len(t, comments.Comments, 0)
				assert.Equal(t, 0, comments.TotalCount)
				assert.Equal(t, DiscussionCommentListDirectionForward, comments.Direction)
			},
		},
		{
			name:   "first page newest reverses comments",
			limit:  5,
			newest: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionWithComments\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false,
										"comments": {
											"totalCount": 8,
											"pageInfo": {"endCursor": "", "hasNextPage": false, "startCursor": "CUR_START", "hasPreviousPage": true},
											"nodes": [
												{
													"id": "C4",
													"url": "",
													"author": {"__typename": "User", "login": "alice"},
													"body": "Fourth",
													"createdAt": "2025-01-04T00:00:00Z",
													"isAnswer": false,
													"upvoteCount": 0,
													"reactionGroups": [],
													"replies": {"totalCount": 0, "nodes": []}
												},
												{
													"id": "C5",
													"url": "",
													"author": {"__typename": "User", "login": "bob"},
													"body": "Fifth",
													"createdAt": "2025-01-05T00:00:00Z",
													"isAnswer": false,
													"upvoteCount": 0,
													"reactionGroups": [],
													"replies": {"totalCount": 0, "nodes": []}
												}
											]
										}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				comments := d.Comments
				assert.Len(t, comments.Comments, 2)
				assert.Equal(t, 8, comments.TotalCount)
				assert.Equal(t, "", comments.Cursor)
				assert.Equal(t, "CUR_START", comments.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionBackward, comments.Direction)
				assert.Equal(t, "C5", comments.Comments[0].ID, "newest mode should reverse comments")
				assert.Equal(t, "C4", comments.Comments[1].ID)
			},
		},
		{
			name:   "multiple replies on comment",
			limit:  10,
			newest: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`query DiscussionWithComments\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false,
										"comments": {
											"totalCount": 1,
											"pageInfo": {"endCursor": "", "hasNextPage": false, "startCursor": "", "hasPreviousPage": false},
											"nodes": [
												{
													"id": "C1",
													"url": "",
													"author": {"__typename": "User", "login": "alice"},
													"body": "Parent",
													"createdAt": "2025-01-01T00:00:00Z",
													"isAnswer": false,
													"upvoteCount": 0,
													"reactionGroups": [],
													"replies": {
														"totalCount": 3,
														"nodes": [
															{
																"id": "R1",
																"url": "",
																"author": {"__typename": "User", "login": "bob"},
																"body": "First reply",
																"createdAt": "2025-01-02T00:00:00Z",
																"isAnswer": false,
																"upvoteCount": 0,
																"reactionGroups": []
															},
															{
																"id": "R2",
																"url": "",
																"author": {"__typename": "User", "login": "carol"},
																"body": "Second reply",
																"createdAt": "2025-01-03T00:00:00Z",
																"isAnswer": false,
																"upvoteCount": 0,
																"reactionGroups": []
															},
															{
																"id": "R3",
																"url": "",
																"author": {"__typename": "User", "login": "dave"},
																"body": "Third reply",
																"createdAt": "2025-01-04T00:00:00Z",
																"isAnswer": false,
																"upvoteCount": 0,
																"reactionGroups": []
															}
														]
													}
												}
											]
										}
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				comments := d.Comments
				assert.Len(t, comments.Comments, 1)
				assert.Equal(t, 1, comments.TotalCount)
				assert.Equal(t, DiscussionCommentListDirectionForward, comments.Direction)
				c := comments.Comments[0]
				require.Len(t, c.Replies.Comments, 3)
				assert.Equal(t, 3, c.Replies.TotalCount)
				assert.Equal(t, "R1", c.Replies.Comments[0].ID)
				assert.Equal(t, "R2", c.Replies.Comments[1].ID)
				assert.Equal(t, "R3", c.Replies.Comments[2].ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			d, err := c.GetWithComments(repo, 1, tt.limit, tt.after, tt.newest)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, d)
			require.NotNil(t, tt.assertDisc, "assertDisc must be set for non-error cases")
			tt.assertDisc(t, d)
		})
	}
}

func TestGetCommentReplies(t *testing.T) {
	tests := []struct {
		name       string
		commentID  string
		limit      int
		after      string
		newest     bool
		httpStubs  func(*testing.T, *httpmock.Registry)
		wantErr    string
		assertDisc func(*testing.T, *Discussion)
	}{
		{
			name:      "maps all fields",
			commentID: "DC_abc",
			limit:     10,
			newest:    false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCommentReplies\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"node": {
									"id": "DC_abc",
									"url": "https://github.com/OWNER/REPO/discussions/42#discussioncomment-1",
									"author": {"__typename": "User", "login": "octocat", "id": "U_octocat", "name": "Octocat"},
									"body": "Top-level comment",
									"createdAt": "2025-03-01T00:00:00Z",
									"isAnswer": true,
									"upvoteCount": 5,
									"reactionGroups": [{"content": "HEART", "users": {"totalCount": 2}}],
									"discussion": {
										"id": "D_1",
										"number": 42,
										"title": "Test Discussion",
										"body": "Discussion body",
										"url": "https://github.com/OWNER/REPO/discussions/42",
										"closed": true,
										"stateReason": "RESOLVED",
										"isAnswered": true,
										"answerChosenAt": "2025-06-01T12:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U_alice", "name": "Alice"},
										"category": {"id": "CAT1", "name": "Q&A", "slug": "q-a", "emoji": ":question:", "isAnswerable": true},
										"answerChosenBy": {"__typename": "User", "login": "bob", "id": "U_bob", "name": "Bob"},
										"labels": {"nodes": [{"id": "L1", "name": "bug", "color": "d73a4a"}]},
										"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 3}}],
										"createdAt": "2025-01-01T00:00:00Z",
										"updatedAt": "2025-01-02T00:00:00Z",
										"closedAt": "2025-06-01T00:00:00Z",
										"locked": true
									},
									"replies": {
										"totalCount": 1,
										"pageInfo": {"endCursor": "REP_CUR", "hasNextPage": true, "startCursor": "REP_START", "hasPreviousPage": false},
										"nodes": [
											{
												"id": "R1",
												"url": "https://github.com/OWNER/REPO/discussions/42#discussioncomment-2",
												"author": {"__typename": "User", "login": "hubot", "id": "U_hubot", "name": "Hubot"},
												"body": "A reply",
												"createdAt": "2025-04-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 1,
												"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 1}}]
											}
										]
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				assert.Equal(t, Discussion{
					ID:          "D_1",
					Number:      42,
					Title:       "Test Discussion",
					Body:        "Discussion body",
					URL:         "https://github.com/OWNER/REPO/discussions/42",
					Closed:      true,
					StateReason: "RESOLVED",
					Author:      DiscussionActor{ID: "U_alice", Login: "alice", Name: "Alice"},
					Category: DiscussionCategory{
						ID:           "CAT1",
						Name:         "Q&A",
						Slug:         "q-a",
						Emoji:        ":question:",
						IsAnswerable: true,
					},
					Labels:         []DiscussionLabel{{ID: "L1", Name: "bug", Color: "d73a4a"}},
					Answered:       true,
					AnswerChosenAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
					AnswerChosenBy: &DiscussionActor{ID: "U_bob", Login: "bob", Name: "Bob"},
					ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 3}},
					CreatedAt:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt:      time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
					ClosedAt:       time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
					Locked:         true,
					Comments: DiscussionCommentList{
						TotalCount: 1,
						Comments: []DiscussionComment{
							{
								ID:             "DC_abc",
								URL:            "https://github.com/OWNER/REPO/discussions/42#discussioncomment-1",
								Author:         DiscussionActor{ID: "U_octocat", Login: "octocat", Name: "Octocat"},
								Body:           "Top-level comment",
								CreatedAt:      time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
								IsAnswer:       true,
								UpvoteCount:    5,
								ReactionGroups: []ReactionGroup{{Content: "HEART", TotalCount: 2}},
								Replies: DiscussionCommentList{
									TotalCount: 1,
									NextCursor: "REP_CUR",
									Direction:  DiscussionCommentListDirectionForward,
									Comments: []DiscussionComment{
										{
											ID:             "R1",
											URL:            "https://github.com/OWNER/REPO/discussions/42#discussioncomment-2",
											Author:         DiscussionActor{ID: "U_hubot", Login: "hubot", Name: "Hubot"},
											Body:           "A reply",
											CreatedAt:      time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
											UpvoteCount:    1,
											ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 1}},
										},
									},
								},
							},
						},
					},
				}, *d)
			},
		},
		{
			name:      "pagination forward oldest",
			commentID: "DC_abc",
			limit:     5,
			after:     "CUR_A",
			newest:    false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCommentReplies\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"node": {
									"id": "DC_abc",
									"url": "",
									"author": {"__typename": "User", "login": "alice"},
									"body": "Comment",
									"createdAt": "2025-01-01T00:00:00Z",
									"isAnswer": false,
									"upvoteCount": 0,
									"reactionGroups": [],
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									},
									"replies": {
										"totalCount": 3,
										"pageInfo": {"endCursor": "CUR_B", "hasNextPage": true, "startCursor": "CUR_A", "hasPreviousPage": false},
										"nodes": [
											{
												"id": "R1",
												"url": "",
												"author": {"__typename": "User", "login": "bob"},
												"body": "Reply 1",
												"createdAt": "2025-02-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 0,
												"reactionGroups": []
											},
											{
												"id": "R2",
												"url": "",
												"author": {"__typename": "User", "login": "carol"},
												"body": "Reply 2",
												"createdAt": "2025-03-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 0,
												"reactionGroups": []
											}
										]
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				replies := d.Comments.Comments[0].Replies
				assert.Len(t, replies.Comments, 2)
				assert.Equal(t, 3, replies.TotalCount)
				assert.Equal(t, "CUR_A", replies.Cursor)
				assert.Equal(t, "CUR_B", replies.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionForward, replies.Direction)
				assert.Equal(t, "R1", replies.Comments[0].ID, "forward mode should preserve chronological order")
				assert.Equal(t, "R2", replies.Comments[1].ID)
			},
		},
		{
			name:      "pagination backward newest reverses replies",
			commentID: "DC_abc",
			limit:     5,
			after:     "CUR_X",
			newest:    true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCommentReplies\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"node": {
									"id": "DC_abc",
									"url": "",
									"author": {"__typename": "User", "login": "alice"},
									"body": "Comment",
									"createdAt": "2025-01-01T00:00:00Z",
									"isAnswer": false,
									"upvoteCount": 0,
									"reactionGroups": [],
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									},
									"replies": {
										"totalCount": 5,
										"pageInfo": {"endCursor": "CUR_END", "hasNextPage": false, "startCursor": "CUR_Y", "hasPreviousPage": true},
										"nodes": [
											{
												"id": "R1",
												"url": "",
												"author": {"__typename": "User", "login": "bob"},
												"body": "Older",
												"createdAt": "2025-02-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 0,
												"reactionGroups": []
											},
											{
												"id": "R2",
												"url": "",
												"author": {"__typename": "User", "login": "carol"},
												"body": "Newer",
												"createdAt": "2025-03-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 0,
												"reactionGroups": []
											}
										]
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				replies := d.Comments.Comments[0].Replies
				assert.Len(t, replies.Comments, 2)
				assert.Equal(t, 5, replies.TotalCount)
				assert.Equal(t, "CUR_X", replies.Cursor)
				assert.Equal(t, "CUR_Y", replies.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionBackward, replies.Direction)
				assert.Equal(t, "R2", replies.Comments[0].ID, "newest mode should reverse replies")
				assert.Equal(t, "R1", replies.Comments[1].ID)
			},
		},
		{
			name:      "first page newest reverses replies",
			commentID: "DC_abc",
			limit:     5,
			newest:    true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCommentReplies\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"node": {
									"id": "DC_abc",
									"url": "",
									"author": {"__typename": "User", "login": "alice"},
									"body": "Comment",
									"createdAt": "2025-01-01T00:00:00Z",
									"isAnswer": false,
									"upvoteCount": 0,
									"reactionGroups": [],
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									},
									"replies": {
										"totalCount": 3,
										"pageInfo": {"endCursor": "", "hasNextPage": false, "startCursor": "CUR_START", "hasPreviousPage": true},
										"nodes": [
											{
												"id": "R1",
												"url": "",
												"author": {"__typename": "User", "login": "bob"},
												"body": "Older",
												"createdAt": "2025-02-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 0,
												"reactionGroups": []
											},
											{
												"id": "R2",
												"url": "",
												"author": {"__typename": "User", "login": "carol"},
												"body": "Newer",
												"createdAt": "2025-03-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 0,
												"reactionGroups": []
											}
										]
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				replies := d.Comments.Comments[0].Replies
				assert.Len(t, replies.Comments, 2)
				assert.Equal(t, 3, replies.TotalCount)
				assert.Equal(t, "", replies.Cursor)
				assert.Equal(t, "CUR_START", replies.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionBackward, replies.Direction)
				assert.Equal(t, "R2", replies.Comments[0].ID, "newest mode should reverse replies")
				assert.Equal(t, "R1", replies.Comments[1].ID)
			},
		},
		{
			name:      "no more pages",
			commentID: "DC_abc",
			limit:     10,
			newest:    false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCommentReplies\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"node": {
									"id": "DC_abc",
									"url": "",
									"author": {"__typename": "User", "login": "alice"},
									"body": "Comment",
									"createdAt": "2025-01-01T00:00:00Z",
									"isAnswer": false,
									"upvoteCount": 0,
									"reactionGroups": [],
									"discussion": {
										"id": "D_1",
										"number": 1,
										"title": "Test",
										"body": "",
										"url": "",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice"},
										"category": {"id": "C1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2024-01-01T00:00:00Z",
										"updatedAt": "2024-01-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									},
									"replies": {
										"totalCount": 1,
										"pageInfo": {"endCursor": "CUR_ONLY", "hasNextPage": false, "startCursor": "CUR_ONLY", "hasPreviousPage": false},
										"nodes": [
											{
												"id": "R1",
												"url": "",
												"author": {"__typename": "User", "login": "bob"},
												"body": "Only reply",
												"createdAt": "2025-02-01T00:00:00Z",
												"isAnswer": false,
												"upvoteCount": 0,
												"reactionGroups": []
											}
										]
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: func(t *testing.T, d *Discussion) {
				replies := d.Comments.Comments[0].Replies
				assert.Len(t, replies.Comments, 1)
				assert.Equal(t, 1, replies.TotalCount)
				assert.Equal(t, "", replies.NextCursor)
				assert.Equal(t, DiscussionCommentListDirectionForward, replies.Direction)
			},
		},
		{
			name:      "reply node not found",
			commentID: "DC_invalid",
			limit:     10,
			newest:    false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCommentReplies\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"node": null
							},
							"errors": [
								{
									"type": "NOT_FOUND",
									"path": ["node"],
									"message": "Could not resolve to a node with the global id of 'DC_invalid'"
								}
							]
						}
					`)),
				)
			},
			wantErr: "Could not resolve to a node",
		},
		{
			name:      "node is not a discussion comment",
			commentID: "I_notacomment",
			limit:     10,
			newest:    false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query DiscussionCommentReplies\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"node": {}
							}
						}
					`)),
				)
			},
			wantErr: "node I_notacomment is not a discussion comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			d, err := c.GetCommentReplies("github.com", tt.commentID, tt.limit, tt.after, tt.newest)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, d)
			require.Len(t, d.Comments.Comments, 1, "GetCommentReplies should return exactly one comment")
			require.NotNil(t, tt.assertDisc, "assertDisc must be set for non-error cases")
			tt.assertDisc(t, d)
		})
	}
}

func repoMetaResp(id string, discussionsEnabled bool) string {
	return fmt.Sprintf(`{
		"data": {
			"repository": {
				"id": %q,
				"databaseId": 982069338,
				"hasDiscussionsEnabled": %t
			}
		}
	}`, id, discussionsEnabled)
}

func TestCreate(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name       string
		input      CreateDiscussionInput
		httpStubs  func(*testing.T, *httpmock.Registry)
		wantErr    string
		assertDisc *Discussion
	}{
		{
			name: "maps all fields",
			input: CreateDiscussionInput{
				CategoryID: "CAT_1",
				Title:      "New Discussion",
				Body:       "Discussion body",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation CreateDiscussion\b`, func(input map[string]interface{}) bool {
						assert.Equal(t, "R_1", input["repositoryId"])
						assert.Equal(t, "CAT_1", input["categoryId"])
						assert.Equal(t, "New Discussion", input["title"])
						assert.Equal(t, "Discussion body", input["body"])
						return true
					}),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"createDiscussion": {
									"discussion": {
										"id": "D_new",
										"number": 99,
										"title": "New Discussion",
										"body": "Discussion body",
										"url": "https://github.com/OWNER/REPO/discussions/99",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": ":speech_balloon:", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 0}}],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: &Discussion{
				ID:     "D_new",
				Number: 99,
				Title:  "New Discussion",
				Body:   "Discussion body",
				URL:    "https://github.com/OWNER/REPO/discussions/99",
				Author: DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:    "CAT_1",
					Name:  "General",
					Slug:  "general",
					Emoji: ":speech_balloon:",
				},
				Labels:         []DiscussionLabel{},
				ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 0}},
				CreatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "discussions disabled",
			input: CreateDiscussionInput{
				CategoryID: "CAT_1",
				Title:      "Test",
				Body:       "Body",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", false)),
				)
			},
			wantErr: "has discussions disabled",
		},
		{
			name: "repo not found",
			input: CreateDiscussionInput{
				CategoryID: "CAT_1",
				Title:      "Test",
				Body:       "Body",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": null
							},
							"errors": [
								{
									"type": "NOT_FOUND",
									"path": ["repository"],
									"message": "Could not resolve to a Repository with the name 'OWNER/REPO'."
								}
							]
						}
					`)),
				)
			},
			wantErr: "Could not resolve to a Repository with the name 'OWNER/REPO'.",
		},
		{
			name: "mutation error",
			input: CreateDiscussionInput{
				CategoryID: "BAD_CAT",
				Title:      "Test",
				Body:       "Body",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CreateDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"createDiscussion": null
							},
							"errors": [
								{
									"type": "NOT_FOUND",
									"message": "Could not resolve to a node with the global id of 'BAD_CAT'."
								}
							]
						}
					`)),
				)
			},
			wantErr: "Could not resolve to a node with the global id of 'BAD_CAT'.",
		},
		{
			name: "creates discussion with labels via addLabels mutation",
			input: CreateDiscussionInput{
				CategoryID: "CAT_1",
				Title:      "New Discussion",
				Body:       "Discussion body",
				LabelIDs:   []string{"L_bug", "L_enh"},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CreateDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"createDiscussion": {
									"discussion": {
										"id": "D_new",
										"number": 99,
										"title": "New Discussion",
										"body": "Discussion body",
										"url": "https://github.com/OWNER/REPO/discussions/99",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": ":speech_balloon:", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 0}}],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation AddLabelsToDiscussion\b`, func(input map[string]interface{}) bool {
						assert.Equal(t, "D_new", input["labelableId"])
						labelIDs, ok := input["labelIds"].([]interface{})
						assert.True(t, ok)
						assert.Equal(t, []interface{}{"L_bug", "L_enh"}, labelIDs)
						return true
					}),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"addLabelsToLabelable": {
									"labelable": {
										"id": "D_new",
										"number": 99,
										"title": "New Discussion",
										"body": "Discussion body",
										"url": "https://github.com/OWNER/REPO/discussions/99",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": ":speech_balloon:", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {
											"nodes": [
												{"id": "L_bug", "name": "bug", "color": "d73a4a"},
												{"id": "L_enh", "name": "enhancement", "color": "a2eeef"}
											]
										},
										"reactionGroups": [{"content": "THUMBS_UP","users": {"totalCount": 0}}],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: &Discussion{
				ID:     "D_new",
				Number: 99,
				Title:  "New Discussion",
				Body:   "Discussion body",
				URL:    "https://github.com/OWNER/REPO/discussions/99",
				Author: DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:    "CAT_1",
					Name:  "General",
					Slug:  "general",
					Emoji: ":speech_balloon:",
				},
				Labels: []DiscussionLabel{
					{ID: "L_bug", Name: "bug", Color: "d73a4a"},
					{ID: "L_enh", Name: "enhancement", Color: "a2eeef"},
				},
				ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 0}},
				CreatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "add labels mutation failure returns discussion and error",
			input: CreateDiscussionInput{
				CategoryID: "CAT_1",
				Title:      "Test",
				Body:       "Body",
				LabelIDs:   []string{"L_bug"},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
				reg.Register(
					httpmock.GraphQL(`mutation CreateDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"createDiscussion": {
									"discussion": {
										"id": "D_new",
										"number": 99,
										"title": "Test",
										"body": "Body",
										"url": "https://github.com/OWNER/REPO/discussions/99",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": ":speech_balloon:", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
				reg.Register(
					httpmock.GraphQL(`mutation AddLabelsToDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": null,
							"errors": [{"message": "could not apply labels"}]
						}
					`)),
				)
			},
			wantErr: "discussion created but some mutations failed: GraphQL: could not apply labels",
			assertDisc: &Discussion{
				ID:     "D_new",
				Number: 99,
				Title:  "Test",
				Body:   "Body",
				URL:    "https://github.com/OWNER/REPO/discussions/99",
				Author: DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:    "CAT_1",
					Name:  "General",
					Slug:  "general",
					Emoji: ":speech_balloon:",
				},
				Labels:    []DiscussionLabel{},
				CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			d, err := c.Create(repo, tt.input)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				if tt.assertDisc != nil {
					require.NotNil(t, d)
					assert.Equal(t, tt.assertDisc, d)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, d)
			require.NotNil(t, tt.assertDisc, "assertDisc must be set for non-error cases")
			assert.Equal(t, tt.assertDisc, d)
		})
	}
}

func TestListLabels(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		want      []DiscussionLabel
		wantErr   string
	}{
		{
			name: "single page",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLabelsForDiscussions\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"labels": {
										"nodes": [
											{"id": "L_bug", "name": "bug", "color": "d73a4a"},
											{"id": "L_enh", "name": "enhancement", "color": "a2eeef"}
										],
										"pageInfo": {"hasNextPage": false, "endCursor": ""}
									}
								}
							}
						}
					`)),
				)
			},
			want: []DiscussionLabel{
				{ID: "L_bug", Name: "bug", Color: "d73a4a"},
				{ID: "L_enh", Name: "enhancement", Color: "a2eeef"},
			},
		},
		{
			name: "multiple pages",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLabelsForDiscussions\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"labels": {
										"nodes": [
											{"id": "L_bug", "name": "bug", "color": "d73a4a"}
										],
										"pageInfo": {"hasNextPage": true, "endCursor": "CUR_1"}
									}
								}
							}
						}
					`)),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryLabelsForDiscussions\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"labels": {
										"nodes": [
											{"id": "L_enh", "name": "enhancement", "color": "a2eeef"}
										],
										"pageInfo": {"hasNextPage": false, "endCursor": ""}
									}
								}
							}
						}
					`)),
				)
			},
			want: []DiscussionLabel{
				{ID: "L_bug", Name: "bug", Color: "d73a4a"},
				{ID: "L_enh", Name: "enhancement", Color: "a2eeef"},
			},
		},
		{
			name: "empty repository",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLabelsForDiscussions\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"repository": {
									"labels": {
										"nodes": [],
										"pageInfo": {"hasNextPage": false, "endCursor": ""}
									}
								}
							}
						}
					`)),
				)
			},
			want: nil,
		},
		{
			name: "query error",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLabelsForDiscussions\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"something went wrong"}]}`),
				)
			},
			wantErr: "something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			tt.httpStubs(reg)

			client := newTestDiscussionClient(reg).(*discussionClient)
			labels, err := client.ListLabels(repo)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, labels)
		})
	}
}

func TestEditDiscussionLabels(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	baseNode := func() discussionListNode {
		return discussionListNode{
			ID:     "D_1",
			Number: 5,
			Title:  "T",
			Body:   "B",
			URL:    "https://github.com/OWNER/REPO/discussions/5",
			Author: actorNode{
				TypeName: "User",
				Login:    "alice",
				User:     struct{ ID, Name string }{ID: "U1", Name: "Alice"},
				Bot:      struct{ ID string }{ID: "U1"},
			},
			Category: struct {
				ID           string
				Name         string
				Slug         string
				Emoji        string
				IsAnswerable bool
			}{ID: "CAT_1", Name: "General", Slug: "general"},
			ReactionGroups: []struct {
				Content string
				Users   struct{ TotalCount int }
			}{},
			CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		}
	}

	tests := []struct {
		name      string
		addIDs    []string
		removeIDs []string
		setupMock func(reg *httpmock.Registry)
		wantErr   string
		wantNode  func() discussionListNode
	}{
		{
			name:      "adds and removes labels",
			addIDs:    []string{"L_bug", "L_enh"},
			removeIDs: []string{"L_old"},
			setupMock: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation RemoveLabelsFromDiscussion\b`, func(input map[string]interface{}) bool {
						assert.Equal(t, "D_1", input["labelableId"])
						assert.Equal(t, []interface{}{"L_old"}, input["labelIds"])
						return true
					}),
					// This response is superseded by the subsequent add mutation so we don't need all fields.
					httpmock.StringResponse(`{"data":{"removeLabelsFromLabelable":{"labelable":{"id": "D_1"}}}}`),
				)
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation AddLabelsToDiscussion\b`, func(input map[string]interface{}) bool {
						assert.Equal(t, "D_1", input["labelableId"])
						assert.Equal(t, []interface{}{"L_bug", "L_enh"}, input["labelIds"])
						return true
					}),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"addLabelsToLabelable": {
									"labelable": {
										"id": "D_1",
										"number": 5,
										"title": "T",
										"body": "B",
										"url": "https://github.com/OWNER/REPO/discussions/5",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {
											"nodes": [
												{"id": "L_bug", "name": "bug", "color": "d73a4a"},
												{"id": "L_enh", "name": "enhancement", "color": "a2eeef"}
											]
										},
										"reactionGroups": [],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
			},
			wantNode: func() discussionListNode {
				n := baseNode()
				n.Labels.Nodes = []struct {
					ID    string
					Name  string
					Color string
				}{
					{ID: "L_bug", Name: "bug", Color: "d73a4a"},
					{ID: "L_enh", Name: "enhancement", Color: "a2eeef"},
				}
				return n
			},
		},
		{
			name:      "only adds labels",
			addIDs:    []string{"L_bug"},
			removeIDs: nil,
			setupMock: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation AddLabelsToDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"addLabelsToLabelable": {
									"labelable": {
										"id": "D_1",
										"number": 5,
										"title": "T",
										"body": "B",
										"url": "https://github.com/OWNER/REPO/discussions/5",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {
											"nodes": [
												{"id": "L_bug", "name": "bug", "color": "d73a4a"}
											]
										},
										"reactionGroups": [],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
			},
			wantNode: func() discussionListNode {
				n := baseNode()
				n.Labels.Nodes = []struct {
					ID    string
					Name  string
					Color string
				}{
					{ID: "L_bug", Name: "bug", Color: "d73a4a"},
				}
				return n
			},
		},
		{
			name:      "only removes labels",
			addIDs:    nil,
			removeIDs: []string{"L_old"},
			setupMock: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation RemoveLabelsFromDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"removeLabelsFromLabelable": {
									"labelable": {
										"id": "D_1",
										"number": 5,
										"title": "T",
										"body": "B",
										"url": "https://github.com/OWNER/REPO/discussions/5",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": "", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {
											"nodes": []
										},
										"reactionGroups": [],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-01T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
			},
			wantNode: func() discussionListNode {
				n := baseNode()
				n.Labels.Nodes = []struct {
					ID    string
					Name  string
					Color string
				}{}
				return n
			},
		},
		{
			name:      "skips both when empty",
			addIDs:    nil,
			removeIDs: nil,
			setupMock: func(reg *httpmock.Registry) {},
		},
		{
			name:      "remove error stops before add",
			addIDs:    []string{"L_bug"},
			removeIDs: []string{"L_old"},
			setupMock: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation RemoveLabelsFromDiscussion\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"could not remove labels"}]}`),
				)
			},
			wantErr: "could not remove labels",
		},
		{
			name:      "add error is returned",
			addIDs:    []string{"L_bug"},
			removeIDs: nil,
			setupMock: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation AddLabelsToDiscussion\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"could not add labels"}]}`),
				)
			},
			wantErr: "could not add labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			tt.setupMock(reg)

			client := newTestDiscussionClient(reg).(*discussionClient)

			node, err := client.editDiscussionLabels(repo, "D_1", tt.addIDs, tt.removeIDs)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.wantNode == nil {
				assert.Nil(t, node)
			} else {
				require.NotNil(t, node)
				assert.Equal(t, tt.wantNode(), *node)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	titleStr := "Updated title"
	bodyStr := "Updated body"
	catID := "CAT_2"

	tests := []struct {
		name       string
		input      UpdateDiscussionInput
		httpStubs  func(*testing.T, *httpmock.Registry)
		wantErr    string
		assertDisc *Discussion
	}{
		{
			name: "nothing to update",
			input: UpdateDiscussionInput{
				DiscussionID: "D_1",
			},
			wantErr: "nothing to update",
		},
		{
			name: "maps all fields",
			input: UpdateDiscussionInput{
				DiscussionID:   "D_1",
				Title:          &titleStr,
				Body:           &bodyStr,
				CategoryID:     &catID,
				AddLabelIDs:    []string{"L_bug", "L_enh"},
				RemoveLabelIDs: []string{"L_old", "L_stale"},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation UpdateDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"updateDiscussion": {
									"discussion": {
										"id": "D_1",
										"number": 5,
										"title": "Updated title",
										"body": "Updated body",
										"url": "https://github.com/OWNER/REPO/discussions/5",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_2", "name": "Q&A", "slug": "q-a", "emoji": ":question:", "isAnswerable": true},
										"answerChosenBy": null,
										"labels": {"nodes": [{"id": "L_bug", "name": "bug", "color": "d73a4a"}, {"id": "L_enh", "name": "enhancement", "color": "a2eeef"}]},
										"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 0}}],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-02T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
				reg.Register(
					httpmock.GraphQL(`mutation RemoveLabelsFromDiscussion\b`),
					httpmock.StringResponse(`{"data":{"removeLabelsFromLabelable":{"labelable":{"id":"D_1","number":5,"title":"Updated title","body":"Updated body","url":"https://github.com/OWNER/REPO/discussions/5","closed":false,"stateReason":"","isAnswered":false,"answerChosenAt":"0001-01-01T00:00:00Z","author":{"__typename":"User","login":"alice","id":"U1","name":"Alice"},"category":{"id":"CAT_2","name":"Q&A","slug":"q-a","emoji":":question:","isAnswerable":true},"answerChosenBy":null,"labels":{"nodes":[]},"reactionGroups":[{"content":"THUMBS_UP","users":{"totalCount":0}}],"createdAt":"2025-06-01T00:00:00Z","updatedAt":"2025-06-02T00:00:00Z","closedAt":"0001-01-01T00:00:00Z","locked":false}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation AddLabelsToDiscussion\b`),
					httpmock.StringResponse(`{"data":{"addLabelsToLabelable":{"labelable":{"id":"D_1","number":5,"title":"Updated title","body":"Updated body","url":"https://github.com/OWNER/REPO/discussions/5","closed":false,"stateReason":"","isAnswered":false,"answerChosenAt":"0001-01-01T00:00:00Z","author":{"__typename":"User","login":"alice","id":"U1","name":"Alice"},"category":{"id":"CAT_2","name":"Q&A","slug":"q-a","emoji":":question:","isAnswerable":true},"answerChosenBy":null,"labels":{"nodes":[{"id":"L_bug","name":"bug","color":"d73a4a"},{"id":"L_enh","name":"enhancement","color":"a2eeef"}]},"reactionGroups":[{"content":"THUMBS_UP","users":{"totalCount":0}}],"createdAt":"2025-06-01T00:00:00Z","updatedAt":"2025-06-02T00:00:00Z","closedAt":"0001-01-01T00:00:00Z","locked":false}}}}`),
				)
			},
			assertDisc: &Discussion{
				ID:     "D_1",
				Number: 5,
				Title:  "Updated title",
				Body:   "Updated body",
				URL:    "https://github.com/OWNER/REPO/discussions/5",
				Author: DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:           "CAT_2",
					Name:         "Q&A",
					Slug:         "q-a",
					Emoji:        ":question:",
					IsAnswerable: true,
				},
				Labels:         []DiscussionLabel{{ID: "L_bug", Name: "bug", Color: "d73a4a"}, {ID: "L_enh", Name: "enhancement", Color: "a2eeef"}},
				ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 0}},
				CreatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "partial update title only",
			input: UpdateDiscussionInput{
				DiscussionID: "D_1",
				Title:        &titleStr,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation UpdateDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"updateDiscussion": {
									"discussion": {
										"id": "D_1",
										"number": 5,
										"title": "Updated title",
										"body": "Original body",
										"url": "https://github.com/OWNER/REPO/discussions/5",
										"closed": false,
										"stateReason": "",
										"isAnswered": false,
										"answerChosenAt": "0001-01-01T00:00:00Z",
										"author": {"__typename": "User", "login": "alice", "id": "U1", "name": "Alice"},
										"category": {"id": "CAT_1", "name": "General", "slug": "general", "emoji": ":speech_balloon:", "isAnswerable": false},
										"answerChosenBy": null,
										"labels": {"nodes": []},
										"reactionGroups": [],
										"createdAt": "2025-06-01T00:00:00Z",
										"updatedAt": "2025-06-02T00:00:00Z",
										"closedAt": "0001-01-01T00:00:00Z",
										"locked": false
									}
								}
							}
						}
					`)),
				)
			},
			assertDisc: &Discussion{
				ID:     "D_1",
				Number: 5,
				Title:  "Updated title",
				Body:   "Original body",
				URL:    "https://github.com/OWNER/REPO/discussions/5",
				Author: DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:    "CAT_1",
					Name:  "General",
					Slug:  "general",
					Emoji: ":speech_balloon:",
				},
				Labels:    []DiscussionLabel{},
				CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "mutation error",
			input: UpdateDiscussionInput{
				DiscussionID: "D_1",
				Title:        &titleStr,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation UpdateDiscussion\b`),
					httpmock.StringResponse(heredoc.Doc(`
						{
							"data": {
								"updateDiscussion": null
							},
							"errors": [
								{
									"type": "NOT_FOUND",
									"message": "Could not resolve to a Discussion with the global id of 'D_1'."
								}
							]
						}
					`)),
				)
			},
			wantErr: "Could not resolve to a Discussion with the global id of 'D_1'.",
		},
		{
			name: "label only update",
			input: UpdateDiscussionInput{
				DiscussionID:   "D_1",
				AddLabelIDs:    []string{"L_bug", "L_enh"},
				RemoveLabelIDs: []string{"L_old", "L_stale"},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation RemoveLabelsFromDiscussion\b`),
					httpmock.StringResponse(`{"data":{"removeLabelsFromLabelable":{"labelable":{"id":"D_1","number":5,"title":"T","body":"B","url":"https://github.com/OWNER/REPO/discussions/5","closed":false,"stateReason":"","isAnswered":false,"answerChosenAt":"0001-01-01T00:00:00Z","author":{"__typename":"User","login":"alice","id":"U1","name":"Alice"},"category":{"id":"CAT_1","name":"General","slug":"general","emoji":"","isAnswerable":false},"answerChosenBy":null,"labels":{"nodes":[]},"reactionGroups":[],"createdAt":"2025-06-01T00:00:00Z","updatedAt":"2025-06-01T00:00:00Z","closedAt":"0001-01-01T00:00:00Z","locked":false}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation AddLabelsToDiscussion\b`),
					httpmock.StringResponse(`{"data":{"addLabelsToLabelable":{"labelable":{"id":"D_1","number":5,"title":"T","body":"B","url":"https://github.com/OWNER/REPO/discussions/5","closed":false,"stateReason":"","isAnswered":false,"answerChosenAt":"0001-01-01T00:00:00Z","author":{"__typename":"User","login":"alice","id":"U1","name":"Alice"},"category":{"id":"CAT_1","name":"General","slug":"general","emoji":"","isAnswerable":false},"answerChosenBy":null,"labels":{"nodes":[{"id":"L_bug","name":"bug","color":"d73a4a"},{"id":"L_enh","name":"enhancement","color":"a2eeef"}]},"reactionGroups":[],"createdAt":"2025-06-01T00:00:00Z","updatedAt":"2025-06-01T00:00:00Z","closedAt":"0001-01-01T00:00:00Z","locked":false}}}}`),
				)
			},
			assertDisc: &Discussion{
				ID:     "D_1",
				Number: 5,
				Title:  "T",
				Body:   "B",
				URL:    "https://github.com/OWNER/REPO/discussions/5",
				Author: DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:   "CAT_1",
					Name: "General",
					Slug: "general",
				},
				Labels:    []DiscussionLabel{{ID: "L_bug", Name: "bug", Color: "d73a4a"}, {ID: "L_enh", Name: "enhancement", Color: "a2eeef"}},
				CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "label failure after field update returns discussion and error",
			input: UpdateDiscussionInput{
				DiscussionID: "D_1",
				Title:        &titleStr,
				AddLabelIDs:  []string{"L_bug"},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation UpdateDiscussion\b`),
					httpmock.StringResponse(`{"data":{"updateDiscussion":{"discussion":{"id":"D_1","number":5,"title":"Updated title","body":"B","url":"https://github.com/OWNER/REPO/discussions/5","closed":false,"stateReason":"","isAnswered":false,"answerChosenAt":"0001-01-01T00:00:00Z","author":{"__typename":"User","login":"alice","id":"U1","name":"Alice"},"category":{"id":"CAT_1","name":"General","slug":"general","emoji":"","isAnswerable":false},"answerChosenBy":null,"labels":{"nodes":[]},"reactionGroups":[],"createdAt":"2025-06-01T00:00:00Z","updatedAt":"2025-06-01T00:00:00Z","closedAt":"0001-01-01T00:00:00Z","locked":false}}}}`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation AddLabelsToDiscussion\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"could not apply labels"}]}`),
				)
			},
			wantErr: "discussion updated but some mutations failed: GraphQL: could not apply labels",
			assertDisc: &Discussion{
				ID:     "D_1",
				Number: 5,
				Title:  "Updated title",
				Body:   "B",
				URL:    "https://github.com/OWNER/REPO/discussions/5",
				Author: DiscussionActor{ID: "U1", Login: "alice", Name: "Alice"},
				Category: DiscussionCategory{
					ID:   "CAT_1",
					Name: "General",
					Slug: "general",
				},
				Labels:    []DiscussionLabel{},
				CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "label only failure returns nil discussion and error",
			input: UpdateDiscussionInput{
				DiscussionID: "D_1",
				AddLabelIDs:  []string{"L_bug"},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation AddLabelsToDiscussion\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"could not apply labels"}]}`),
				)
			},
			wantErr: "could not apply labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			c := newTestDiscussionClient(reg)
			d, err := c.Update(repo, tt.input)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				if tt.assertDisc != nil {
					require.NotNil(t, d)
					assert.Equal(t, tt.assertDisc, d)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, d)
			require.NotNil(t, tt.assertDisc, "assertDisc must be set for non-error cases")
			assert.Equal(t, tt.assertDisc, d)
		})
	}
}

func TestAddComment(t *testing.T) {
	tests := []struct {
		name         string
		discussionID string
		body         string
		replyToID    string
		httpStubs    func(*testing.T, *httpmock.Registry)
		wantErr      string
		wantComment  *DiscussionComment
	}{
		{
			name:         "adds top-level comment",
			discussionID: "D_123",
			body:         "Hello world",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation AddDiscussionComment\b`, func(input map[string]interface{}) bool {
						assert.Equal(t, "D_123", input["discussionId"])
						assert.Equal(t, "Hello world", input["body"])
						assert.Nil(t, input["replyToId"])
						return true
					}),
					httpmock.StringResponse(`{
						"data": {
							"addDiscussionComment": {
								"comment": {
									"id": "DC_1",
									"url": "https://github.com/OWNER/REPO/discussions/1#discussioncomment-1",
									"author": {"__typename": "User", "login": "monalisa", "id": "U1", "name": "Mona"},
									"body": "Hello world",
									"createdAt": "2025-06-01T00:00:00Z",
									"isAnswer": false,
									"upvoteCount": 1,
									"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 0}}]
								}
							}
						}
					}`),
				)
			},
			wantComment: &DiscussionComment{
				ID:             "DC_1",
				URL:            "https://github.com/OWNER/REPO/discussions/1#discussioncomment-1",
				Author:         DiscussionActor{ID: "U1", Login: "monalisa", Name: "Mona"},
				Body:           "Hello world",
				CreatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpvoteCount:    1,
				ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 0}},
			},
		},
		{
			name:         "adds reply to comment",
			discussionID: "D_123",
			body:         "Reply text",
			replyToID:    "DC_parent",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation AddDiscussionComment\b`, func(input map[string]interface{}) bool {
						assert.Equal(t, "D_123", input["discussionId"])
						assert.Equal(t, "Reply text", input["body"])
						assert.Equal(t, "DC_parent", input["replyToId"])
						return true
					}),
					httpmock.StringResponse(`{
						"data": {
							"addDiscussionComment": {
								"comment": {
									"id": "DC_reply",
									"url": "https://github.com/OWNER/REPO/discussions/1#discussioncomment-2",
									"author": {"__typename": "User", "login": "monalisa", "id": "U1", "name": "Mona"},
									"body": "Reply text",
									"createdAt": "2025-06-01T00:00:00Z",
									"isAnswer": false,
									"upvoteCount": 1,
									"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 0}}]
								}
							}
						}
					}`),
				)
			},
			wantComment: &DiscussionComment{
				ID:             "DC_reply",
				URL:            "https://github.com/OWNER/REPO/discussions/1#discussioncomment-2",
				Author:         DiscussionActor{ID: "U1", Login: "monalisa", Name: "Mona"},
				Body:           "Reply text",
				CreatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpvoteCount:    1,
				ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 0}},
			},
		},
		{
			name:         "mutation error",
			discussionID: "D_bad",
			body:         "text",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation AddDiscussionComment\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"not found"}]}`),
				)
			},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			repo := ghrepo.New("OWNER", "REPO")
			c := newTestDiscussionClient(reg)
			comment, err := c.AddComment(repo, tt.discussionID, tt.body, tt.replyToID)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, comment)
			assert.Equal(t, tt.wantComment, comment)
		})
	}
}

func TestUpdateComment(t *testing.T) {
	tests := []struct {
		name        string
		commentID   string
		body        string
		httpStubs   func(*testing.T, *httpmock.Registry)
		wantErr     string
		wantComment *DiscussionComment
	}{
		{
			name:      "updates comment body",
			commentID: "DC_1",
			body:      "Updated body",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQLMutationMatcher(`mutation UpdateDiscussionComment\b`, func(input map[string]interface{}) bool {
						assert.Equal(t, "DC_1", input["commentId"])
						assert.Equal(t, "Updated body", input["body"])
						return true
					}),
					httpmock.StringResponse(`{
						"data": {
							"updateDiscussionComment": {
								"comment": {
									"id": "DC_1",
									"url": "https://github.com/OWNER/REPO/discussions/1#discussioncomment-1",
									"author": {"__typename": "User", "login": "monalisa", "id": "U1", "name": "Mona"},
									"body": "Updated body",
									"createdAt": "2025-06-01T00:00:00Z",
									"isAnswer": true,
									"upvoteCount": 5,
									"reactionGroups": [{"content": "HEART", "users": {"totalCount": 3}}]
								}
							}
						}
					}`),
				)
			},
			wantComment: &DiscussionComment{
				ID:             "DC_1",
				URL:            "https://github.com/OWNER/REPO/discussions/1#discussioncomment-1",
				Author:         DiscussionActor{ID: "U1", Login: "monalisa", Name: "Mona"},
				Body:           "Updated body",
				CreatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				IsAnswer:       true,
				UpvoteCount:    5,
				ReactionGroups: []ReactionGroup{{Content: "HEART", TotalCount: 3}},
			},
		},
		{
			name:      "mutation error",
			commentID: "DC_bad",
			body:      "text",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation UpdateDiscussionComment\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"not found"}]}`),
				)
			},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			repo := ghrepo.New("OWNER", "REPO")
			c := newTestDiscussionClient(reg)
			comment, err := c.UpdateComment(repo, tt.commentID, tt.body)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, comment)
			assert.Equal(t, tt.wantComment, comment)
		})
	}
}

func TestDeleteComment(t *testing.T) {
	tests := []struct {
		name      string
		commentID string
		httpStubs func(*httpmock.Registry)
		wantErr   string
	}{
		{
			name:      "deletes comment",
			commentID: "DC_1",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation DeleteDiscussionComment\b`),
					httpmock.StringResponse(`{"data":{"deleteDiscussionComment":{"comment":{"id":"DC_1"}}}}`),
				)
			},
		},
		{
			name:      "mutation error",
			commentID: "DC_bad",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation DeleteDiscussionComment\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"not found"}]}`),
				)
			},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			repo := ghrepo.New("OWNER", "REPO")
			c := newTestDiscussionClient(reg)
			err := c.DeleteComment(repo, tt.commentID)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGetComment(t *testing.T) {
	tests := []struct {
		name        string
		commentID   string
		httpStubs   func(*httpmock.Registry)
		wantErr     string
		wantComment *DiscussionComment
	}{
		{
			name:      "fetches comment",
			commentID: "DC_1",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query GetDiscussionComment\b`),
					httpmock.StringResponse(`{
						"data": {
							"node": {
								"__typename": "DiscussionComment",
								"id": "DC_1",
								"url": "https://github.com/OWNER/REPO/discussions/1#discussioncomment-1",
								"author": {"__typename": "User", "login": "monalisa", "id": "U1", "name": "Mona"},
								"body": "Comment body",
								"createdAt": "2025-06-01T00:00:00Z",
								"isAnswer": false,
								"upvoteCount": 2,
								"reactionGroups": [{"content": "THUMBS_UP", "users": {"totalCount": 1}}]
							}
						}
					}`),
				)
			},
			wantComment: &DiscussionComment{
				ID:             "DC_1",
				URL:            "https://github.com/OWNER/REPO/discussions/1#discussioncomment-1",
				Author:         DiscussionActor{ID: "U1", Login: "monalisa", Name: "Mona"},
				Body:           "Comment body",
				CreatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				UpvoteCount:    2,
				ReactionGroups: []ReactionGroup{{Content: "THUMBS_UP", TotalCount: 1}},
			},
		},
		{
			name:      "wrong node type",
			commentID: "I_123",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query GetDiscussionComment\b`),
					httpmock.StringResponse(`{
						"data": {
							"node": {
								"__typename": "Issue"
							}
						}
					}`),
				)
			},
			wantErr: "is not a discussion comment (got Issue)",
		},
		{
			name:      "not found",
			commentID: "DC_bad",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query GetDiscussionComment\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"Could not resolve to a node"}]}`),
				)
			},
			wantErr: "Could not resolve to a node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			repo := ghrepo.New("OWNER", "REPO")
			c := newTestDiscussionClient(reg)
			comment, err := c.GetComment(repo.RepoHost(), tt.commentID)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, comment)
			assert.Equal(t, tt.wantComment, comment)
		})
	}
}

func TestResolveCommentNodeID(t *testing.T) {
	tests := []struct {
		name              string
		commentDatabaseID int64
		httpStubs         func(*httpmock.Registry)
		wantNodeID        string
		wantErr           string
	}{
		{
			name:              "encodes node ID correctly",
			commentDatabaseID: 17196842,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(repoMetaResp("R_1", true)),
				)
			},
			wantNodeID: "DC_kwDOOokwWs4BBmcq",
		},
		{
			name:              "repo not found",
			commentDatabaseID: 123,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMetaForDiscussions\b`),
					httpmock.StringResponse(`{"data":null,"errors":[{"message":"repo not found"}]}`),
				)
			},
			wantErr: "repo not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := ghrepo.New("OWNER", "REPO")
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			c := newTestDiscussionClient(reg)
			nodeID, err := c.ResolveCommentNodeID(repo, tt.commentDatabaseID)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantNodeID, nodeID)
		})
	}
}
