package list

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func newTestClient(reg *httpmock.Registry) *api.Client {
	client := &http.Client{}
	httpmock.ReplaceTripper(client, reg)
	return api.NewClientFromHTTP(client)
}

func getterFactory(r []string, err error) GetSponsorsList {
	return func(*http.Client, string, string) ([]string, error) {
		return r, err
	}
}

func TestSponsorsListGetter(t *testing.T) {

	var tests = []struct {
		name           string
		get            GetSponsorsList
		expectedResult []string
		expectError    bool
	}{
		{
			name:           "hello world",
			get:            getterFactory([]string{"hello world"}, nil),
			expectedResult: []string{"hello world"},
			expectError:    false,
		},
		{
			name:           "error",
			get:            getterFactory([]string{}, fmt.Errorf("error")),
			expectedResult: []string{},
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sponsorsListGetter := NewSponsorsListGetter(tt.get)
			sponsorsList, err := sponsorsListGetter.get(nil, "", "")
			if err != nil && !tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
			assert.Equal(t, tt.expectedResult, sponsorsList)
		})
	}
}

func Test_querySponsors(t *testing.T) {
	var tests = []struct {
		name           string
		httpStubs      func(*httpmock.Registry)
		username       string
		expectedResult SponsorQuery
		expectError    bool
	}{
		{
			name: "success",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SponsorsList`),
					httpmock.GraphQLQuery(`{"data": {"user": {"sponsors": {"totalCount": 2, "nodes": [{"login": "mona"}, {"login": "lisa"}]}}}}`,
						func(query string, vars map[string]interface{}) {
							want := map[string]interface{}{"username": "octocat"}
							if !reflect.DeepEqual(vars, want) {
								t.Errorf("got variables %v, want %v", vars, want)
							}
						},
					),
				)
			},
			username: "octocat",
			expectedResult: SponsorQuery{
				User: User{
					Sponsors: Sponsors{
						TotalCount: 2,
						Nodes: []Node{
							{Login: "mona"},
							{Login: "lisa"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "different username",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SponsorsList`),
					httpmock.GraphQLQuery(`{"data": {"user": {"sponsors": {"totalCount": 2, "nodes": [{"login": "octo"}, {"login": "cat"}]}}}}`,
						func(query string, vars map[string]interface{}) {
							want := map[string]interface{}{"username": "monalisa"}
							if !reflect.DeepEqual(vars, want) {
								t.Errorf("got variables %v, want %v", vars, want)
							}
						},
					),
				)
			},
			username: "monalisa",
			expectedResult: SponsorQuery{
				User: User{
					Sponsors: Sponsors{
						TotalCount: 2,
						Nodes: []Node{
							{Login: "octo"},
							{Login: "cat"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "error",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SponsorsList`),
					httpmock.StatusStringResponse(500, `Internal Server Error`))
			},
			username:       "octocat",
			expectedResult: SponsorQuery{},
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf(tt.name), func(t *testing.T) {
			http := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(http)
			}

			client := newTestClient(http)

			sponsorsList, err := querySponsors(client, "", tt.username)
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}

			assert.Equal(t, tt.expectedResult, sponsorsList)
		})
	}
}

func Test_mapQueryToSponsorsList(t *testing.T) {
	var tests = []struct {
		name           string
		queryData      SponsorQuery
		expectedResult []string
	}{
		{
			name:           "empty query data",
			queryData:      SponsorQuery{},
			expectedResult: []string{},
		},
		{
			name: "query data with sponsors",
			queryData: SponsorQuery{
				User: User{
					Sponsors: Sponsors{
						TotalCount: 2,
						Nodes: []Node{
							{Login: "mona"},
							{Login: "lisa"},
						},
					},
				},
			},
			expectedResult: []string{"mona", "lisa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedResult, mapQueryToSponsorsList(tt.queryData))
		})
	}
}
