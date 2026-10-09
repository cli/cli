package client

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPullRequest(t *testing.T) {
	tests := []struct {
		name     string
		repo     ghrepo.Interface
		response string
		want     bool
		wantURL  string
		wantErr  string
	}{
		{
			name:     "an issue",
			repo:     ghrepo.New("monalisa", "monas-cafe"),
			response: `{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"__typename":"Issue","number":142}}}}`,
			want:     false,
			wantURL:  "https://api.github.com/graphql",
		},
		{
			name:     "a pull request",
			repo:     ghrepo.New("monalisa", "monas-cafe"),
			response: `{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"__typename":"PullRequest","number":142}}}}`,
			want:     true,
			wantURL:  "https://api.github.com/graphql",
		},
		{
			name:     "a repository on a ghe.com host",
			repo:     ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			response: `{"data":{"repository":{"hasIssuesEnabled":true,"issue":{"__typename":"Issue","number":142}}}}`,
			want:     false,
			wantURL:  "https://api.monas-cafe.ghe.com/graphql",
		},
		{
			name: "a number that is neither",
			repo: ghrepo.New("monalisa", "monas-cafe"),
			response: `{"data":{"repository":{"hasIssuesEnabled":true,"issue":null}},"errors":[{"type":"NOT_FOUND",` +
				`"message":"Could not resolve to an issue or pull request with the number of 142.","path":["repository","issue"]}]}`,
			wantURL: "https://api.github.com/graphql",
			wantErr: "GraphQL: Could not resolve to an issue or pull request with the number of 142. (repository.issue)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.GraphQL(`query IssueByNumber\b`),
				httpmock.GraphQLQuery(tt.response, func(_ string, vars map[string]any) {
					assert.Equal(t, "monalisa", vars["owner"])
					assert.Equal(t, "monas-cafe", vars["repo"])
					assert.Equal(t, float64(142), vars["number"])
				}),
			)

			c := NewArtifactClient(&http.Client{Transport: reg})
			got, err := c.IsPullRequest(tt.repo, 142)

			require.Len(t, reg.Requests, 1)
			assert.Equal(t, tt.wantURL, reg.Requests[0].URL.String())
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestList(t *testing.T) {
	const listURL = "https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts"
	const tenancyListURL = "https://api.monas-cafe.ghe.com/repos/monalisa/monas-cafe/issues/142/artifacts"

	createdAt := time.Date(2026, 9, 21, 23, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 9, 24, 21, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		repo         ghrepo.Interface
		artifactType string
		limit        int
		pages        []httpmock.Responder
		want         []Artifact
		wantNumbers  []int
		wantURLs     []string
		wantErr      string
	}{
		{
			name: "decodes every field, and null users and timestamps",
			repo: ghrepo.New("monalisa", "monas-cafe"),
			pages: []httpmock.Responder{
				httpmock.StringResponse(`[
					{
						"id": 6626,
						"number": 2,
						"type": "generic",
						"name": "OAuth callback plan",
						"body": "# OAuth callback plan",
						"body_html": "<h1>OAuth callback plan</h1>",
						"description": "Register the callback URL.",
						"creator": {"login": "hubot", "id": 2},
						"updated_by_actor": {"login": "monalisa", "id": 1},
						"created_at": "2026-09-21T23:00:00Z",
						"updated_at": "2026-09-24T21:00:00Z"
					},
					{
						"id": 6627,
						"number": 3,
						"type": "link",
						"name": "Staging OAuth runbook",
						"body": "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook",
						"body_html": "",
						"description": "",
						"creator": null,
						"updated_by_actor": null,
						"created_at": null,
						"updated_at": null
					}
				]`),
			},
			want: []Artifact{
				{
					ID:             6626,
					Number:         2,
					Type:           "generic",
					Name:           "OAuth callback plan",
					Body:           "# OAuth callback plan",
					BodyHTML:       "<h1>OAuth callback plan</h1>",
					Description:    "Register the callback URL.",
					Creator:        &Actor{ID: 2, Login: "hubot"},
					UpdatedByActor: &Actor{ID: 1, Login: "monalisa"},
					CreatedAt:      &createdAt,
					UpdatedAt:      &updatedAt,
				},
				{
					ID:     6627,
					Number: 3,
					Type:   "link",
					Name:   "Staging OAuth runbook",
					Body:   "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook",
				},
			},
			wantURLs: []string{listURL + "?per_page=100"},
		},
		{
			name:        "stops at a page that comes back short",
			repo:        ghrepo.New("monalisa", "monas-cafe"),
			pages:       []httpmock.Responder{artifactsPage(2, 3, 5)},
			wantNumbers: []int{2, 3, 5},
			wantURLs:    []string{listURL + "?per_page=100"},
		},
		{
			name:        "requests the next page after a full one",
			repo:        ghrepo.New("monalisa", "monas-cafe"),
			pages:       []httpmock.Responder{artifactsPage(numbers(1, 100)...), artifactsPage(101)},
			wantNumbers: numbers(1, 101),
			wantURLs: []string{
				listURL + "?per_page=100",
				listURL + "?page=2&per_page=100",
			},
		},
		{
			name:        "a full last page is followed by an empty one",
			repo:        ghrepo.New("monalisa", "monas-cafe"),
			pages:       []httpmock.Responder{artifactsPage(numbers(1, 100)...), artifactsPage()},
			wantNumbers: numbers(1, 100),
			wantURLs: []string{
				listURL + "?per_page=100",
				listURL + "?page=2&per_page=100",
			},
		},
		{
			name:        "no artifacts",
			repo:        ghrepo.New("monalisa", "monas-cafe"),
			pages:       []httpmock.Responder{artifactsPage()},
			wantNumbers: []int{},
			wantURLs:    []string{listURL + "?per_page=100"},
		},
		{
			name:        "a limit below the page size asks for that many",
			repo:        ghrepo.New("monalisa", "monas-cafe"),
			limit:       2,
			pages:       []httpmock.Responder{artifactsPage(2, 3)},
			wantNumbers: []int{2, 3},
			wantURLs:    []string{listURL + "?per_page=2"},
		},
		{
			name:        "a limit of one full page makes one request",
			repo:        ghrepo.New("monalisa", "monas-cafe"),
			limit:       100,
			pages:       []httpmock.Responder{artifactsPage(numbers(1, 100)...)},
			wantNumbers: numbers(1, 100),
			wantURLs:    []string{listURL + "?per_page=100"},
		},
		{
			name:        "a limit above the page size stops partway through a page",
			repo:        ghrepo.New("monalisa", "monas-cafe"),
			limit:       150,
			pages:       []httpmock.Responder{artifactsPage(numbers(1, 100)...), artifactsPage(numbers(101, 200)...)},
			wantNumbers: numbers(1, 150),
			wantURLs: []string{
				listURL + "?per_page=100",
				listURL + "?page=2&per_page=100",
			},
		},
		{
			name:         "a type filter",
			repo:         ghrepo.New("monalisa", "monas-cafe"),
			artifactType: "link",
			pages:        []httpmock.Responder{artifactsPage(3)},
			wantNumbers:  []int{3},
			wantURLs:     []string{listURL + "?per_page=100&type=link"},
		},
		{
			name:         "a type filter on a later page",
			repo:         ghrepo.New("monalisa", "monas-cafe"),
			artifactType: "generic",
			pages:        []httpmock.Responder{artifactsPage(numbers(1, 100)...), artifactsPage(101)},
			wantNumbers:  numbers(1, 101),
			wantURLs: []string{
				listURL + "?per_page=100&type=generic",
				listURL + "?page=2&per_page=100&type=generic",
			},
		},
		{
			name:        "a repository on a ghe.com host",
			repo:        ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			pages:       []httpmock.Responder{artifactsPage(2, 3, 5)},
			wantNumbers: []int{2, 3, 5},
			wantURLs:    []string{tenancyListURL + "?per_page=100"},
		},
		{
			name:     "an API error is returned as the API gives it",
			repo:     ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			pages:    []httpmock.Responder{httpmock.StatusJSONResponse(503, map[string]string{"message": "Artifact storage is not available"})},
			wantURLs: []string{tenancyListURL + "?per_page=100"},
			wantErr:  "HTTP 503: Artifact storage is not available (" + tenancyListURL + "?per_page=100)",
		},
		{
			name: "an API error on a later page",
			repo: ghrepo.New("monalisa", "monas-cafe"),
			pages: []httpmock.Responder{
				artifactsPage(numbers(1, 100)...),
				httpmock.StatusJSONResponse(404, map[string]string{"message": "Not Found"}),
			},
			wantURLs: []string{
				listURL + "?per_page=100",
				listURL + "?page=2&per_page=100",
			},
			wantErr: "HTTP 404: Not Found (" + listURL + "?page=2&per_page=100)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			for _, page := range tt.pages {
				reg.Register(httpmock.REST("GET", "repos/monalisa/monas-cafe/issues/142/artifacts"), page)
			}

			c := NewArtifactClient(&http.Client{Transport: reg})
			got, err := c.List(tt.repo, 142, tt.artifactType, tt.limit)

			var gotURLs []string
			for _, req := range reg.Requests {
				gotURLs = append(gotURLs, req.URL.String())
			}
			assert.Equal(t, tt.wantURLs, gotURLs)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			if tt.want != nil {
				assert.Equal(t, tt.want, got)
				return
			}
			gotNumbers := []int{}
			for _, a := range got {
				gotNumbers = append(gotNumbers, a.Number)
			}
			assert.Equal(t, tt.wantNumbers, gotNumbers)
		})
	}
}

// artifactsPage responds with a page holding an artifact for each number.
func artifactsPage(artifactNumbers ...int) httpmock.Responder {
	page := make([]map[string]any, 0, len(artifactNumbers))
	for _, n := range artifactNumbers {
		page = append(page, map[string]any{
			"id":     6600 + n,
			"number": n,
			"type":   "generic",
			"name":   fmt.Sprintf("Artifact %d", n),
		})
	}
	return httpmock.JSONResponse(page)
}

// numbers returns the numbers from first to last.
func numbers(first, last int) []int {
	n := make([]int, 0, last-first+1)
	for i := first; i <= last; i++ {
		n = append(n, i)
	}
	return n
}
