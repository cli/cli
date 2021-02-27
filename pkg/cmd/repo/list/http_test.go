package list

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/cli/cli/pkg/httpmock"
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
	assert.Equal(t, `sort:updated-desc user:@me fork:true language:"go"`, searchData.Variables["query"])
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
			want: "sort:updated-desc user:@me fork:true",
		},
		{
			name: "in org",
			args: args{
				owner: "cli",
			},
			want: "sort:updated-desc user:cli fork:true",
		},
		{
			name: "only public",
			args: args{
				owner: "",
				filter: FilterOptions{
					Visibility: "public",
				},
			},
			want: "sort:updated-desc user:@me fork:true is:public",
		},
		{
			name: "only private",
			args: args{
				owner: "",
				filter: FilterOptions{
					Visibility: "private",
				},
			},
			want: "sort:updated-desc user:@me fork:true is:private",
		},
		{
			name: "only forks",
			args: args{
				owner: "",
				filter: FilterOptions{
					Fork: true,
				},
			},
			want: "sort:updated-desc user:@me fork:only",
		},
		{
			name: "no forks",
			args: args{
				owner: "",
				filter: FilterOptions{
					Source: true,
				},
			},
			want: "sort:updated-desc user:@me fork:false",
		},
		{
			name: "with language",
			args: args{
				owner: "",
				filter: FilterOptions{
					Language: "ruby",
				},
			},
			want: "sort:updated-desc user:@me fork:true language:\"ruby\"",
		},
		{
			name: "only archived",
			args: args{
				owner: "",
				filter: FilterOptions{
					Archived: true,
				},
			},
			want: "sort:updated-desc user:@me fork:true archived:true",
		},
		{
			name: "only non-archived",
			args: args{
				owner: "",
				filter: FilterOptions{
					NonArchived: true,
				},
			},
			want: "sort:updated-desc user:@me fork:true archived:false",
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
