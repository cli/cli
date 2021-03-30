package list

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/httpmock"
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
		httpStub func(*httpmock.Registry)
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
			httpStub: func(r *httpmock.Registry) {
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
			httpStub: func(r *httpmock.Registry) {
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
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestSearch\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"q":     `repo:OWNER/REPO is:pr is:open label:hello label:"one world" sort:created-desc`,
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
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestSearch\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"q":     "repo:OWNER/REPO is:pr is:open author:monalisa",
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
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestSearch\b`),
					httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
						want := map[string]interface{}{
							"q":     "repo:OWNER/REPO is:pr is:open one world in:title",
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
				tt.httpStub(reg)
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
