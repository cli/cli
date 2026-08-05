package githubrest_test

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

func TestBranchDeleteRemote(t *testing.T) {
	var tests = []struct {
		name        string
		branch      string
		httpStubs   func(*httpmock.Registry)
		expectError bool
	}{
		{
			name:   "success",
			branch: "owner/branch#123",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads%2Fowner%2Fbranch%23123"),
					httpmock.StatusStringResponse(204, ""))
			},
			expectError: false,
		},
		{
			name:   "error",
			branch: "my-branch",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads%2Fmy-branch"),
					httpmock.StatusStringResponse(500, `{"message": "oh no"}`))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			client, err := httpmock.RESTClientFunc(reg)("github.com")
			require.NoError(t, err)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			err = githubrest.BranchDeleteRemote(context.Background(), client, repo, tt.branch)
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}
