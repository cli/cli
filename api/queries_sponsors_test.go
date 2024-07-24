package api

import (
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_querySponsorsViaReflection(t *testing.T) {
	var tests = []struct {
		name           string
		httpStubs      func(*httpmock.Registry)
		expectedResult SponsorQuery
		expectError    bool
	}{
		{
			name: "success",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SponsorsList`),
					httpmock.StatusStringResponse(200, `{"data": {"user": {"sponsors": {"totalCount": 2, "nodes": [{"login": "mona"}, {"login": "lisa"}]}}}}`))
			},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(http)
			}

			client := newTestClient(http)

			sponsorsList, err := querySponsorsViaReflection(client, "github.com", "octocat")
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
