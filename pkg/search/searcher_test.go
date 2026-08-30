package search

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/MakeNowJust/heredoc"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearcherCode(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "code",
		Limit:    30,
		Qualifiers: Qualifiers{
			Language: "go",
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"q":        []string{"keyword language:go"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    CodeResult
		wantErr   bool
		errMsg    string
		httpStubs func(reg *httpmock.Registry)
	}{
		{
			name:  "searches code",
			query: query,
			result: CodeResult{
				IncompleteResults: false,
				Items:             []Code{{Name: "file.go"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/code", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"name": "file.go",
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches code for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: CodeResult{
				IncompleteResults: false,
				Items:             []Code{{Name: "file.go"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/code", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"name": "file.go",
							},
						},
					}),
				)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: CodeResult{
				IncompleteResults: false,
				Items:             []Code{{Name: "file.go"}, {Name: "file2.go"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/code", values)
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "file.go",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/code?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"q":        []string{"keyword language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "file2.go",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "paginates results with quoted multi-word query (#11228)",
			query: Query{
				Keywords: []string{"keyword with whitespace"},
				Kind:     "code",
				Limit:    30,
				Qualifiers: Qualifiers{
					Language: "go",
				},
			},
			result: CodeResult{
				IncompleteResults: false,
				Items:             []Code{{Name: "file.go"}, {Name: "file2.go"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"30"},
					"q":        []string{"\"keyword with whitespace\" language:go"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "file.go",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/code?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"q":        []string{"\"keyword with whitespace\" language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "file2.go",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "code",
				Limit:    110,
				Qualifiers: Qualifiers{
					Language: "go",
				},
			},
			result: CodeResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Code {
					return Code{
						Name: fmt.Sprintf("name%d.go", i),
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"q":        []string{"keyword language:go"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) any {
						return map[string]any{
							"name": fmt.Sprintf("name%d.go", i),
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/code?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"q":        []string{"keyword language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) any {
						return map[string]any{
							"name": fmt.Sprintf("name%d.go", i),
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword language:go".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/code", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
								"message": "Validation Failed",
								"errors": [
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://developer.github.com/v3/search/"
							}`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host, &fd.DisabledDetectorMock{})
			result, err := searcher.Code(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherCommits(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "commits",
		Limit:    30,
		Order:    "desc",
		Sort:     "committer-date",
		Qualifiers: Qualifiers{
			Author:        "foobar",
			CommitterDate: ">2021-02-28",
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"order":    []string{"desc"},
		"sort":     []string{"committer-date"},
		"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    CommitsResult
		wantErr   bool
		errMsg    string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name:  "searches commits",
			query: query,
			result: CommitsResult{
				IncompleteResults: false,
				Items:             []Commit{{Sha: "abc"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/commits", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"sha": "abc",
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches commits for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: CommitsResult{
				IncompleteResults: false,
				Items:             []Commit{{Sha: "abc"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/commits", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"sha": "abc",
							},
						},
					}),
				)
			},
		},
		{
			name: "paginates results with quoted multi-word query (#11228)",
			query: Query{
				Keywords: []string{"keyword with whitespace"},
				Kind:     "commits",
				Limit:    30,
				Order:    "desc",
				Sort:     "committer-date",
				Qualifiers: Qualifiers{
					Author:        "foobar",
					CommitterDate: ">2021-02-28",
				},
			},
			result: CommitsResult{
				IncompleteResults: false,
				Items:             []Commit{{Sha: "abc"}, {Sha: "def"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"\"keyword with whitespace\" author:foobar committer-date:>2021-02-28"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"sha": "abc",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/commits?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"\"keyword with whitespace\" author:foobar committer-date:>2021-02-28"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"sha": "def",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: CommitsResult{
				IncompleteResults: false,
				Items:             []Commit{{Sha: "abc"}, {Sha: "def"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/commits", values)
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"sha": "abc",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/commits?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"sha": "def",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "commits",
				Limit:    110,
				Order:    "desc",
				Sort:     "committer-date",
				Qualifiers: Qualifiers{
					Author:        "foobar",
					CommitterDate: ">2021-02-28",
				},
			},
			result: CommitsResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Commit {
					return Commit{
						Sha: strconv.Itoa(i),
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) map[string]any {
						return map[string]any{
							"sha": strconv.Itoa(i),
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/commits?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) map[string]any {
						return map[string]any{
							"sha": strconv.Itoa(i),
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword author:foobar committer-date:>2021-02-28".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/commits", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
                "message":"Validation Failed",
                "errors":[
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://docs.github.com/v3/search/"
              }`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host, &fd.DisabledDetectorMock{})
			result, err := searcher.Commits(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherRepositories(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "repositories",
		Limit:    30,
		Order:    "desc",
		Sort:     "stars",
		Qualifiers: Qualifiers{
			Stars: ">=5",
			Topic: []string{"topic"},
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"order":    []string{"desc"},
		"sort":     []string{"stars"},
		"q":        []string{"keyword stars:>=5 topic:topic"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    RepositoriesResult
		wantErr   bool
		errMsg    string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name:  "searches repositories",
			query: query,
			result: RepositoriesResult{
				IncompleteResults: false,
				Items:             []Repository{{Name: "test"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/repositories", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"name": "test",
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches repositories for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: RepositoriesResult{
				IncompleteResults: false,
				Items:             []Repository{{Name: "test"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/repositories", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"name": "test",
							},
						},
					}),
				)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: RepositoriesResult{
				IncompleteResults: false,
				Items:             []Repository{{Name: "test"}, {Name: "cli"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/repositories", values)
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "test",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/repositories?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"keyword stars:>=5 topic:topic"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "cli",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "paginates results with quoted multi-word query (#11228)",
			query: Query{
				Keywords: []string{"keyword with whitespace"},
				Kind:     "repositories",
				Limit:    30,
				Order:    "desc",
				Sort:     "stars",
				Qualifiers: Qualifiers{
					Stars: ">=5",
					Topic: []string{"topic"},
				},
			},
			result: RepositoriesResult{
				IncompleteResults: false,
				Items:             []Repository{{Name: "test"}, {Name: "cli"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"\"keyword with whitespace\" stars:>=5 topic:topic"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "test",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/repositories?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"\"keyword with whitespace\" stars:>=5 topic:topic"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"name": "cli",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "repositories",
				Limit:    110,
				Order:    "desc",
				Sort:     "stars",
				Qualifiers: Qualifiers{
					Stars: ">=5",
					Topic: []string{"topic"},
				},
			},
			result: RepositoriesResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Repository {
					return Repository{
						Name: fmt.Sprintf("name%d", i),
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"keyword stars:>=5 topic:topic"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) any {
						return map[string]any{
							"name": fmt.Sprintf("name%d", i),
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/repositories?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"keyword stars:>=5 topic:topic"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) any {
						return map[string]any{
							"name": fmt.Sprintf("name%d", i),
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword stars:>=5 topic:topic".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/repositories", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
                "message":"Validation Failed",
                "errors":[
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://docs.github.com/v3/search/"
              }`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host, &fd.DisabledDetectorMock{})
			result, err := searcher.Repositories(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherIssues(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "issues",
		Limit:    30,
		Order:    "desc",
		Sort:     "comments",
		Qualifiers: Qualifiers{
			Language: "go",
			Is:       []string{"public", "locked"},
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"order":    []string{"desc"},
		"sort":     []string{"comments"},
		"q":        []string{"keyword is:locked is:public language:go"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    IssuesResult
		wantErr   bool
		errMsg    string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name:  "searches issues",
			query: query,
			result: IssuesResult{
				IncompleteResults: false,
				Items:             []Issue{{Number: 1234}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"number": 1234,
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches issues for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: IssuesResult{
				IncompleteResults: false,
				Items:             []Issue{{Number: 1234}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/issues", values),
					httpmock.JSONResponse(map[string]any{
						"incomplete_results": false,
						"total_count":        1,
						"items": []any{
							map[string]any{
								"number": 1234,
							},
						},
					}),
				)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: IssuesResult{
				IncompleteResults: false,
				Items:             []Issue{{Number: 1234}, {Number: 5678}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/issues", values)
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"number": 1234,
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/issues?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"keyword is:locked is:public language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"number": 5678,
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "paginates results with quoted multi-word query (#11228)",
			query: Query{
				Keywords: []string{"keyword with whitespace"},
				Kind:     "issues",
				Limit:    30,
				Order:    "desc",
				Sort:     "comments",
				Qualifiers: Qualifiers{
					Language: "go",
					Is:       []string{"public", "locked"},
				},
			},
			result: IssuesResult{
				IncompleteResults: false,
				Items:             []Issue{{Number: 1234}, {Number: 5678}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"\"keyword with whitespace\" is:locked is:public language:go"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"number": 1234,
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/issues?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"\"keyword with whitespace\" is:locked is:public language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        2,
					"items": []any{
						map[string]any{
							"number": 5678,
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "issues",
				Limit:    110,
				Order:    "desc",
				Sort:     "comments",
				Qualifiers: Qualifiers{
					Language: "go",
					Is:       []string{"public", "locked"},
				},
			},
			result: IssuesResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Issue {
					return Issue{
						Number: i,
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"keyword is:locked is:public language:go"},
				})
				firstRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) any {
						return map[string]any{
							"number": i,
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/issues?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"keyword is:locked is:public language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]any{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) any {
						return map[string]any{
							"number": i,
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword is:locked is:public language:go".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
                "message":"Validation Failed",
                "errors":[
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://docs.github.com/v3/search/"
              }`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host, fd.AdvancedIssueSearchUnsupported())
			result, err := searcher.Issues(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherIssuesAdvancedSyntax(t *testing.T) {
	query := Query{
		Kind:     KindIssues,
		Limit:    1,
		Keywords: []string{"keyword"},
		Qualifiers: Qualifiers{
			// Ordinary qualifiers
			Author: "johndoe",
			Label:  []string{"foo", "bar"},
			// Special qualifiers (that should be grouped and OR-ed when using advanced issue search)
			Repo: []string{"foo/bar", "foo/baz"},
			Is:   []string{"private", "public"},
			User: []string{"johndoe", "janedoe"},
			In:   []string{"title", "body", "comments"},
		},
	}

	tests := []struct {
		name       string
		query      Query
		detector   fd.Detector
		wantValues url.Values
		wantErr    string
	}{
		{
			// TODO advancedIssueSearchCleanup
			// Remove this test case once GHES 3.17 support ends.
			name:     "advanced issue search not supported",
			detector: fd.AdvancedIssueSearchUnsupported(),
			query:    query,
			wantValues: url.Values{
				"q":               []string{"keyword author:johndoe in:body in:comments in:title is:private is:public label:bar label:foo repo:foo/bar repo:foo/baz user:janedoe user:johndoe"},
				"advanced_search": nil, // assert absence
			},
		},
		{
			// TODO advancedIssueSearchCleanup
			// Remove this test case once GHES 3.17 support ends.
			name:     "advanced issue search supported as an opt-in feature",
			detector: fd.AdvancedIssueSearchSupportedAsOptIn(),
			query:    query,
			wantValues: url.Values{
				"q":               []string{"( keyword ) author:johndoe (in:body OR in:comments OR in:title) (is:private OR is:public) label:bar label:foo (repo:foo/bar OR repo:foo/baz) (user:janedoe OR user:johndoe)"},
				"advanced_search": []string{"true"}, // opt-in
			},
		},
		{
			// TODO advancedIssueSearchCleanup
			// No need for feature detection once GHES 3.17 support ends.
			name:     "advanced issue search supported as the only search backend",
			detector: fd.AdvancedIssueSearchSupportedAsOnlyBackend(),
			query:    query,
			wantValues: url.Values{
				"q":               []string{"( keyword ) author:johndoe (in:body OR in:comments OR in:title) (is:private OR is:public) label:bar label:foo (repo:foo/bar OR repo:foo/baz) (user:janedoe OR user:johndoe)"},
				"advanced_search": nil, // assert absence
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			reg.Register(
				httpmock.QueryMatcher("GET", "search/issues", tt.wantValues),
				httpmock.JSONResponse(IssuesResult{}),
			)

			client := &http.Client{Transport: reg}
			searcher := NewSearcher(client, "github.com", tt.detector)

			_, err := searcher.Issues(tt.query)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSearcherIssuesSemanticSearch(t *testing.T) {
	tests := []struct {
		name       string
		searchType string
		detector   fd.Detector
		wantValues url.Values
		wantErr    string
	}{
		{
			name:       "semantic search sends search_type=semantic",
			searchType: "semantic",
			detector:   fd.SemanticSearchSupported(),
			wantValues: url.Values{"search_type": []string{"semantic"}},
		},
		{
			name:       "hybrid search sends search_type=hybrid",
			searchType: "hybrid",
			detector:   fd.SemanticSearchSupported(),
			wantValues: url.Values{"search_type": []string{"hybrid"}},
		},
		{
			name:       "lexical search sends no search_type param",
			searchType: "",
			detector:   fd.SemanticSearchSupported(),
			wantValues: url.Values{"search_type": nil}, // assert absence
		},
		{
			name:       "semantic search not supported on host",
			searchType: "semantic",
			detector:   fd.SemanticSearchUnsupported(),
			wantErr:    "semantic search is not supported on this host: github.com",
		},
		{
			name:       "hybrid search not supported on host",
			searchType: "hybrid",
			detector:   fd.SemanticSearchUnsupported(),
			wantErr:    "hybrid search is not supported on this host: github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.wantErr == "" {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", tt.wantValues),
					httpmock.JSONResponse(IssuesResult{}),
				)
			}

			query := Query{
				Kind:            KindIssues,
				Limit:           30,
				Keywords:        []string{"keyword"},
				IssueSearchType: tt.searchType,
			}

			client := &http.Client{Transport: reg}
			searcher := NewSearcher(client, "github.com", tt.detector)

			_, err := searcher.Issues(query)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSearcherIssuesSemanticSearchIsBoundedToSinglePage(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	// The response advertises a next page via the Link header. Only the first
	// page is registered, so if fetching were to paginate it would request an
	// unregistered second page and fail.
	firstRes := httpmock.JSONResponse(map[string]any{
		"incomplete_results": false,
		"total_count":        2,
		"items": []any{
			map[string]any{"number": 1234},
		},
	})
	firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/issues?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
	reg.Register(
		httpmock.QueryMatcher("GET", "search/issues", url.Values{"search_type": []string{"semantic"}}),
		firstRes,
	)

	query := Query{
		Kind:            KindIssues,
		Limit:           100,
		Keywords:        []string{"keyword"},
		IssueSearchType: "semantic",
	}

	client := &http.Client{Transport: reg}
	searcher := NewSearcher(client, "github.com", fd.SemanticSearchSupported())

	result, err := searcher.Issues(query)
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.Items))
}

func TestSearcherURL(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "repositories",
		Limit:    30,
		Order:    "desc",
		Sort:     "stars",
		Qualifiers: Qualifiers{
			Stars: ">=5",
			Topic: []string{"topic"},
		},
	}

	tests := []struct {
		name  string
		host  string
		query Query
		url   string
	}{
		{
			name:  "outputs encoded query url",
			query: query,
			url:   "https://github.com/search?order=desc&q=keyword+stars%3A%3E%3D5+topic%3Atopic&sort=stars&type=repositories",
		},
		{
			name:  "supports enterprise hosts",
			host:  "enterprise.com",
			query: query,
			url:   "https://enterprise.com/search?order=desc&q=keyword+stars%3A%3E%3D5+topic%3Atopic&sort=stars&type=repositories",
		},
		{
			name: "outputs encoded query url with quoted multi-word keywords",
			query: Query{
				Keywords: []string{"keyword with whitespace"},
				Kind:     "repositories",
			},
			url: "https://github.com/search?q=%22keyword+with+whitespace%22&type=repositories",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(nil, tt.host, nil)
			assert.Equal(t, tt.url, searcher.URL(tt.query))
		})
	}
}

// initialize generate slices over a range for test scenarios using the provided initializer.
func initialize[T any](start int, stop int, initializer func(i int) T) []T {
	results := make([]T, 0, (stop - start))
	for i := start; i < stop; i++ {
		results = append(results, initializer(i))
	}
	return results
}
