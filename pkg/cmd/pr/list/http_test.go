package list

import (
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
