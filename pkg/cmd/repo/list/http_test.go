package list

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_listReposWithLanguage(t *testing.T) {
	reg := httpmock.Registry{}
	defer reg.Verify(t)

	var searchData struct {
		Query     string
		Variables map[string]interface{}
	}
	reg.Register(
		httpmock.GraphQL(`query RepositoryListSearch\b`),
		func(req *http.Request) (*http.Response, error) {
			jsonData, err := ioutil.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			err = json.Unmarshal(jsonData, &searchData)
			if err != nil {
				return nil, err
			}

			respBody, err := os.Open("./fixtures/repoSearch.json")
			if err != nil {
				return nil, err
			}

			return &http.Response{
				StatusCode: 200,
				Request:    req,
				Body:       respBody,
			}, nil
		},
	)

	client := http.Client{Transport: &reg}
	res, err := listRepos(&client, "github.com", 10, "", FilterOptions{
		Language: "go",
	})
	require.NoError(t, err)

	assert.Equal(t, 3, res.TotalCount)
	assert.Equal(t, true, res.FromSearch)
	assert.Equal(t, "octocat", res.Owner)
	assert.Equal(t, "octocat/hello-world", res.Repositories[0].NameWithOwner)

	assert.Equal(t, float64(10), searchData.Variables["perPage"])
	assert.Equal(t, `user:@me language:go fork:true sort:updated-desc`, searchData.Variables["query"])
}

func Test_searchQuery(t *testing.T) {
	type args struct {
		owner  string
		filter FilterOptions
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "blank",
			want: "user:@me fork:true sort:updated-desc",
		},
		{
			name: "in org",
			args: args{
				owner: "cli",
			},
			want: "user:cli fork:true sort:updated-desc",
		},
		{
			name: "only public",
			args: args{
				owner: "",
				filter: FilterOptions{
					Visibility: "public",
				},
			},
			want: "user:@me is:public fork:true sort:updated-desc",
		},
		{
			name: "only private",
			args: args{
				owner: "",
				filter: FilterOptions{
					Visibility: "private",
				},
			},
			want: "user:@me is:private fork:true sort:updated-desc",
		},
		{
			name: "only forks",
			args: args{
				owner: "",
				filter: FilterOptions{
					Fork: true,
				},
			},
			want: "user:@me fork:only sort:updated-desc",
		},
		{
			name: "no forks",
			args: args{
				owner: "",
				filter: FilterOptions{
					Source: true,
				},
			},
			want: "user:@me fork:false sort:updated-desc",
		},
		{
			name: "with language",
			args: args{
				owner: "",
				filter: FilterOptions{
					Language: "ruby",
				},
			},
			want: `user:@me language:ruby fork:true sort:updated-desc`,
		},
		{
			name: "only archived",
			args: args{
				owner: "",
				filter: FilterOptions{
					Archived: true,
				},
			},
			want: "user:@me fork:true archived:true sort:updated-desc",
		},
		{
			name: "only non-archived",
			args: args{
				owner: "",
				filter: FilterOptions{
					NonArchived: true,
				},
			},
			want: "user:@me fork:true archived:false sort:updated-desc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchQuery(tt.args.owner, tt.args.filter); got != tt.want {
				t.Errorf("searchQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}
