package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIssuesFromArgs(t *testing.T) {
	tests := []struct {
		behavior             string
		args                 []string
		expectedIssueNumbers []int
		expectedRepo         o.Option[ghrepo.Interface]
		expectedErr          bool
	}{
		{
			behavior:             "when given issue numbers, returns them with no repo",
			args:                 []string{"1", "2"},
			expectedIssueNumbers: []int{1, 2},
			expectedRepo:         o.None[ghrepo.Interface](),
		},
		{
			behavior:             "when given # prefixed issue numbers, returns them with no repo",
			args:                 []string{"#1", "#2"},
			expectedIssueNumbers: []int{1, 2},
			expectedRepo:         o.None[ghrepo.Interface](),
		},
		{
			behavior: "when given URLs, returns them with the repo",
			args: []string{
				"https://github.com/OWNER/REPO/issues/1",
				"https://github.com/OWNER/REPO/issues/2",
			},
			expectedIssueNumbers: []int{1, 2},
			expectedRepo:         o.Some(ghrepo.New("OWNER", "REPO")),
		},
		{
			behavior: "when given URLs in different repos, errors",
			args: []string{
				"https://github.com/OWNER/REPO/issues/1",
				"https://github.com/OWNER/OTHERREPO/issues/2",
			},
			expectedErr: true,
		},
		{
			behavior:    "when given an unparseable argument, errors",
			args:        []string{"://"},
			expectedErr: true,
		},
		{
			behavior:    "when given a URL that isn't an issue or PR url, errors",
			args:        []string{"https://github.com"},
			expectedErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.behavior, func(t *testing.T) {
			issueNumbers, repo, err := ParseIssuesFromArgs(tc.args)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedIssueNumbers, issueNumbers)
			assert.Equal(t, tc.expectedRepo, repo)
		})
	}

}

func TestFindIssuesOrPRs(t *testing.T) {
	tests := []struct {
		name             string
		issueNumbers     []int
		baseRepo         ghrepo.Interface
		httpStub         func(*httpmock.Registry)
		wantIssueNumbers []int
		wantErr          bool
	}{
		{
			name:         "multiple issues",
			issueNumbers: []int{1, 2},
			baseRepo:     ghrepo.New("OWNER", "REPO"),
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":1}
					}}}`))
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
							"hasIssuesEnabled": true,
							"issue":{"number":2}
						}}}`))
			},
			wantIssueNumbers: []int{1, 2},
		},
		{
			name:         "any find error results in total error",
			issueNumbers: []int{1, 2},
			baseRepo:     ghrepo.New("OWNER", "REPO"),
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"number":1}
					}}}`))
				r.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StatusStringResponse(500, "internal server error"))
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStub != nil {
				tt.httpStub(reg)
			}
			httpClient := &http.Client{Transport: reg}
			issues, err := FindIssuesOrPRs(httpClient, tt.baseRepo, tt.issueNumbers, []string{"number"})
			if (err != nil) != tt.wantErr {
				t.Errorf("FindIssuesOrPRs() error = %v, wantErr %v", err, tt.wantErr)
				if issues == nil {
					return
				}
			}
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			for i := range issues {
				assert.Contains(t, tt.wantIssueNumbers, issues[i].Number)
			}
		})
	}
}
