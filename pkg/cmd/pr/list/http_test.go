package list

import (
	"net/http"
	"reflect"
	"testing"

	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_ListPullRequests(t *testing.T) {
	type args struct {
		detector fd.Detector
		repo     ghrepo.Interface
		filters  prShared.FilterOptions
		limit    int
	}
	tests := []struct {
		name     string
		args     args
		httpStub func(*testing.T, *httpmock.Registry)
		wantErr  bool
	}{
		{
			name: "default",
			args: args{
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
				filters: prShared.FilterOptions{
					State: "open",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestList\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"owner": "OWNER",
							"repo":  "REPO",
							"state": []interface{}{"OPEN"},
							"limit": float64(30),
						}
						if !reflect.DeepEqual(vars, want) {
							t.Errorf("got GraphQL variables %#v, want %#v", vars, want)
						}
					}))
			},
		},
		{
			name: "closed",
			args: args{
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
				filters: prShared.FilterOptions{
					State: "closed",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestList\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"owner": "OWNER",
							"repo":  "REPO",
							"state": []interface{}{"CLOSED", "MERGED"},
							"limit": float64(30),
						}
						if !reflect.DeepEqual(vars, want) {
							t.Errorf("got GraphQL variables %#v, want %#v", vars, want)
						}
					}))
			},
		},
		{
			name: "with labels",
			args: args{
				// TODO advancedIssueSearchCleanup
				// No need for feature detection once GHES 3.17 support ends.
				detector: fd.AdvancedIssueSearchSupportedAsOptIn(),
				repo:     ghrepo.New("OWNER", "REPO"),
				limit:    30,
				filters: prShared.FilterOptions{
					State:  "open",
					Labels: []string{"hello", "one world"},
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestSearch\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"q":     `label:"one world" label:hello repo:OWNER/REPO state:open type:pr`,
							"type":  "ISSUE_ADVANCED",
							"limit": float64(30),
						}
						if !reflect.DeepEqual(vars, want) {
							t.Errorf("got GraphQL variables %#v, want %#v", vars, want)
						}
					}))
			},
		},
		{
			name: "with author",
			args: args{
				// TODO advancedIssueSearchCleanup
				// No need for feature detection once GHES 3.17 support ends.
				detector: fd.AdvancedIssueSearchSupportedAsOptIn(),
				repo:     ghrepo.New("OWNER", "REPO"),
				limit:    30,
				filters: prShared.FilterOptions{
					State:  "open",
					Author: "monalisa",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestSearch\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"q":     "author:monalisa repo:OWNER/REPO state:open type:pr",
							"type":  "ISSUE_ADVANCED",
							"limit": float64(30),
						}
						if !reflect.DeepEqual(vars, want) {
							t.Errorf("got GraphQL variables %#v, want %#v", vars, want)
						}
					}))
			},
		},
		{
			name: "with search",
			args: args{
				// TODO advancedIssueSearchCleanup
				// No need for feature detection once GHES 3.17 support ends.
				detector: fd.AdvancedIssueSearchSupportedAsOptIn(),
				repo:     ghrepo.New("OWNER", "REPO"),
				limit:    30,
				filters: prShared.FilterOptions{
					State:  "open",
					Search: "one world in:title",
				},
			},
			httpStub: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestSearch\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"q":     "( one world in:title ) repo:OWNER/REPO state:open type:pr",
							"type":  "ISSUE_ADVANCED",
							"limit": float64(30),
						}
						if !reflect.DeepEqual(vars, want) {
							t.Errorf("got GraphQL variables %#v, want %#v", vars, want)
						}
					}))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStub != nil {
				tt.httpStub(t, reg)
			}
			httpClient := &http.Client{Transport: reg}

			_, err := listPullRequests(httpClient, tt.args.detector, tt.args.repo, tt.args.filters, tt.args.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListPullRequests() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

// TODO advancedIssueSearchCleanup
// Remove this test once GHES 3.17 support ends.
func TestSearchPullRequestsAndAdvancedSearch(t *testing.T) {
	tests := []struct {
		name           string
		detector       fd.Detector
		wantSearchType string
	}{
		{
			name:           "advanced issue search not supported",
			detector:       fd.AdvancedIssueSearchUnsupported(),
			wantSearchType: "ISSUE",
		},
		{
			name:           "advanced issue search supported as opt-in",
			detector:       fd.AdvancedIssueSearchSupportedAsOptIn(),
			wantSearchType: "ISSUE_ADVANCED",
		},
		{
			name:           "advanced issue search supported as only backend",
			detector:       fd.AdvancedIssueSearchSupportedAsOnlyBackend(),
			wantSearchType: "ISSUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			reg.Register(
				httpmock.GraphQL(`query PullRequestSearch\b`),
				httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
					assert.Equal(t, tt.wantSearchType, vars["type"])

					// Since no repeated usage of special search qualifiers is possible
					// with our current implementation, we can assert against the same
					// query for both search backend (i.e. legacy and advanced issue search).
					assert.Equal(t, "repo:OWNER/REPO state:open type:pr", vars["q"])
				}))

			httpClient := &http.Client{Transport: reg}

			searchPullRequests(httpClient, tt.detector, ghrepo.New("OWNER", "REPO"), prShared.FilterOptions{State: "open"}, 30)
		})
	}
}
