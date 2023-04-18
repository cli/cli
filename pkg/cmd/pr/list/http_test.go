package list

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
)

func Test_listPullRequests(t *testing.T) {
	type args struct {
		repo    ghrepo.Interface
		filters prShared.FilterOptions
		limit   int
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
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
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
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
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
				repo:  ghrepo.New("OWNER", "REPO"),
				limit: 30,
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
							"q":     "one world in:title repo:OWNER/REPO state:open type:pr",
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

			_, err := listPullRequests(httpClient, tt.args.repo, tt.args.filters, tt.args.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("listPullRequests() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_ListPRsPaged(t *testing.T) {
	// to create references to
	trueVar := true
	falseVar := false

	type args struct {
		filters prShared.FilterOptions
		limit   int
	}
	type expected struct {
		prCount      int
		totalCount   int
		isUpperBound bool
	}
	tests := []struct {
		name      string
		args      args
		queryType string
		responses []string
		expected  expected
	}{
		{
			name: "default",
			args: args{
				filters: prShared.FilterOptions{
					State: "open",
				},
				limit: 8,
			},
			queryType: "PullRequestList",
			responses: []string{
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 20,
                    "nodes": [
                        {"title": "P1PR1"},
                        {"title": "P1PR2"},
                        {"title": "P1PR3"},
                        {"title": "P1PR4"},
                        {"title": "P1PR5"}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}}`,
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 20,
                    "nodes": [
                        {"title": "P2PR1"},
                        {"title": "P2PR2"},
                        {"title": "P2PR3"}
                    ],
                    "pageInfo": {"hasNextPage": false}
                }}}}`,
			},
			expected: expected{
				prCount:      8,
				totalCount:   20,
				isUpperBound: false,
			},
		},
		{
			name: "with search",
			args: args{
				filters: prShared.FilterOptions{
					State:  "open",
					Search: "one world in:title",
				},
				limit: 30,
			},
			queryType: "PullRequestSearch",
			responses: []string{
				`{"data": {"search": {
                    "issueCount": 20,
                    "nodes": [
                        {"title": "P1PR1"},
                        {"title": "P1PR2"},
                        {"title": "P1PR3"},
                        {"title": "P1PR4"},
                        {"title": "P1PR5"}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}`,
				`{"data": {"search": {
                    "issueCount": 20,
                    "nodes": [
                        {"title": "P2PR1"},
                        {"title": "P2PR2"},
                        {"title": "P2PR3"}
                    ],
                    "pageInfo": {"hasNextPage": false}
                }}}`,
			},
			expected: expected{
				prCount:      8,
				totalCount:   20,
				isUpperBound: false,
			},
		},
		{
			name: "with auto-merge enabled, more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &trueVar,
				},
				limit: 5,
			},
			queryType: "PullRequestList",
			responses: []string{
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 20,
                    "nodes": [
                        {"title": "P1PR1 disabled", "autoMergeRequest": null},
                        {"title": "P1PR2 -- 1 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR3 -- 2 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR4 disabled", "autoMergeRequest": null},
                        {"title": "P1PR5 -- 3 / 5", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}}`,
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 20,
                    "nodes": [
                        {"title": "P2PR1 -- 1 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR2 disabled", "autoMergeRequest": null},
                        {"title": "P2PR3 -- 2 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR4 -- 3 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR5 disabled", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE3"}
                }}}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   16,
				isUpperBound: true,
			},
		},
		{
			name: "with auto-merge enabled, no more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &trueVar,
				},
				limit: 5,
			},
			queryType: "PullRequestList",
			responses: []string{
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 10,
                    "nodes": [
                        {"title": "P1PR1 disabled", "autoMergeRequest": null},
                        {"title": "P1PR2 -- 1 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR3 -- 2 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR4 disabled", "autoMergeRequest": null},
                        {"title": "P1PR5 -- 3 / 5", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}}`,
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 10,
                    "nodes": [
                        {"title": "P2PR1 -- 4 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR2 disabled", "autoMergeRequest": null},
                        {"title": "P2PR3 -- 5 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR4 -- extra", "autoMergeRequest": {}},
                        {"title": "P2PR5 disabled", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": false}
                }}}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   6,
				isUpperBound: false,
			},
		},
		{
			name: "with auto-merge disabled, more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &falseVar,
				},
				limit: 5,
			},
			queryType: "PullRequestList",
			responses: []string{
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 20,
                    "nodes": [
                        {"title": "P1PR1 - 1 / 5", "autoMergeRequest": null},
                        {"title": "P1PR2 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR3 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR4 - 2 / 5", "autoMergeRequest": null},
                        {"title": "P1PR5 enabled", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}}`,
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 20,
                    "nodes": [
                        {"title": "P2PR1 enabled", "autoMergeRequest": {}},
                        {"title": "P2PR2 - 3 / 5", "autoMergeRequest": null},
                        {"title": "P2PR3 - 4 / 5", "autoMergeRequest": null},
                        {"title": "P2PR4 - 5 / 5", "autoMergeRequest": null},
                        {"title": "P2PR5 - extra", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE3"}
                }}}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   16,
				isUpperBound: true,
			},
		},
		{
			name: "with auto-merge disabled, no more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &falseVar,
				},
				limit: 5,
			},
			queryType: "PullRequestList",
			responses: []string{
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 10,
                    "nodes": [
                        {"title": "P1PR1 - 1 / 5", "autoMergeRequest": null},
                        {"title": "P1PR2 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR3 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR4 - 2 / 5", "autoMergeRequest": null},
                        {"title": "P1PR5 enabled", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}}`,
				`{"data": {"repository": {"pullRequests": {
                    "totalCount": 10,
                    "nodes": [
                        {"title": "P2PR1 enabled", "autoMergeRequest": {}},
                        {"title": "P2PR2 - 3 / 5", "autoMergeRequest": null},
                        {"title": "P2PR3 - 4 / 5", "autoMergeRequest": null},
                        {"title": "P2PR4 - 5 / 5", "autoMergeRequest": null},
                        {"title": "P2PR5 - extra", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": false}
                }}}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   6,
				isUpperBound: false,
			},
		},
		{
			name: "with search and auto-merge enabled, more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &trueVar,
					Search:          "one world in:title",
				},
				limit: 5,
			},
			queryType: "PullRequestSearch",
			responses: []string{
				`{"data": {"search": {
                    "issueCount": 20,
                    "nodes": [
                        {"title": "P1PR1 disabled", "autoMergeRequest": null},
                        {"title": "P1PR2 -- 1 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR3 -- 2 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR4 disabled", "autoMergeRequest": null},
                        {"title": "P1PR5 -- 3 / 5", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}`,
				`{"data": {"search": {
                    "issueCount": 20,
                    "nodes": [
                        {"title": "P2PR1 -- 1 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR2 disabled", "autoMergeRequest": null},
                        {"title": "P2PR3 -- 2 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR4 -- 3 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR5 disabled", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE3"}
                }}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   16,
				isUpperBound: true,
			},
		},
		{
			name: "with search and auto-merge enabled, no more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &trueVar,
					Search:          "one world in:title",
				},
				limit: 5,
			},
			queryType: "PullRequestSearch",
			responses: []string{
				`{"data": {"search": {
                    "issueCount": 10,
                    "nodes": [
                        {"title": "P1PR1 disabled", "autoMergeRequest": null},
                        {"title": "P1PR2 -- 1 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR3 -- 2 / 5", "autoMergeRequest": {}},
                        {"title": "P1PR4 disabled", "autoMergeRequest": null},
                        {"title": "P1PR5 -- 3 / 5", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}`,
				`{"data": {"search": {
                    "issueCount": 10,
                    "nodes": [
                        {"title": "P2PR1 -- 4 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR2 disabled", "autoMergeRequest": null},
                        {"title": "P2PR3 -- 5 / 5", "autoMergeRequest": {}},
                        {"title": "P2PR4 -- extra", "autoMergeRequest": {}},
                        {"title": "P2PR5 disabled", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": false}
                }}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   6,
				isUpperBound: false,
			},
		},
		{
			name: "with search and auto-merge disabled, more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &falseVar,
					Search:          "one world in:title",
				},
				limit: 5,
			},
			queryType: "PullRequestSearch",
			responses: []string{
				`{"data": {"search": {
                    "issueCount": 20,
                    "nodes": [
                        {"title": "P1PR1 - 1 / 5", "autoMergeRequest": null},
                        {"title": "P1PR2 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR3 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR4 - 2 / 5", "autoMergeRequest": null},
                        {"title": "P1PR5 enabled", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}`,
				`{"data": {"search": {
                    "issueCount": 20,
                    "nodes": [
                        {"title": "P2PR1 enabled", "autoMergeRequest": {}},
                        {"title": "P2PR2 - 3 / 5", "autoMergeRequest": null},
                        {"title": "P2PR3 - 4 / 5", "autoMergeRequest": null},
                        {"title": "P2PR4 - 5 / 5", "autoMergeRequest": null},
                        {"title": "P2PR5 - extra", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE3"}
                }}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   16,
				isUpperBound: true,
			},
		},
		{
			name: "with search and auto-merge disabled, no more pages",
			args: args{
				filters: prShared.FilterOptions{
					State:           "open",
					AutoMergeStatus: &falseVar,
					Search:          "one world in:title",
				},
				limit: 5,
			},
			queryType: "PullRequestSearch",
			responses: []string{
				`{"data": {"search": {
                    "issueCount": 10,
                    "nodes": [
                        {"title": "P1PR1 - 1 / 5", "autoMergeRequest": null},
                        {"title": "P1PR2 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR3 enabled", "autoMergeRequest": {}},
                        {"title": "P1PR4 - 2 / 5", "autoMergeRequest": null},
                        {"title": "P1PR5 enabled", "autoMergeRequest": {}}
                    ],
                    "pageInfo": {"hasNextPage": true, "endCursor": "PAGE2"}
                }}}`,
				`{"data": {"search": {
                    "issueCount": 10,
                    "nodes": [
                        {"title": "P2PR1 enabled", "autoMergeRequest": {}},
                        {"title": "P2PR2 - 3 / 5", "autoMergeRequest": null},
                        {"title": "P2PR3 - 4 / 5", "autoMergeRequest": null},
                        {"title": "P2PR4 - 5 / 5", "autoMergeRequest": null},
                        {"title": "P2PR5 - extra", "autoMergeRequest": null}
                    ],
                    "pageInfo": {"hasNextPage": false}
                }}}`,
			},
			expected: expected{
				prCount:      5,
				totalCount:   6,
				isUpperBound: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			for _, response := range tt.responses {
				reg.Register(
					httpmock.GraphQL(fmt.Sprintf(`query %s\b`, tt.queryType)),
					httpmock.StringResponse(response),
				)
			}
			httpClient := &http.Client{Transport: reg}

			res, err := listPullRequests(httpClient, ghrepo.New("OWNER", "REPO"), tt.args.filters, tt.args.limit)
			if err != nil {
				t.Errorf("listPullRequests() error = %v", err)
				return
			}
			if len(res.PullRequests) != tt.expected.prCount {
				t.Errorf("listPullRequests() returned %d PRs, want %d", len(res.PullRequests), tt.expected.prCount)
			}
			if res.TotalCount != tt.expected.totalCount {
				t.Errorf("listPullRequests() returned total count %d, want %d", res.TotalCount, tt.expected.totalCount)
			}
			if res.TotalCountIsUpperBound != tt.expected.isUpperBound {
				t.Errorf("listPullRequests() returned total count is upper bound %t, want %t", res.TotalCountIsUpperBound, tt.expected.isUpperBound)
			}
		})
	}
}
