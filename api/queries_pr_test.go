package api

import (
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/httpmock"
)

func TestBranchDeleteRemote(t *testing.T) {
	var tests = []struct {
		name           string
		responseStatus int
		responseBody   string
		expectError    bool
	}{
		{
			name:           "success",
			responseStatus: 204,
			responseBody:   "",
			expectError:    false,
		},
		{
			name:           "error",
			responseStatus: 500,
			responseBody:   `{"message": "oh no"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			http.Register(
				httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/branch"),
				httpmock.StatusStringResponse(tt.responseStatus, tt.responseBody))

			client := NewClient(ReplaceTripper(http))
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			err := BranchDeleteRemote(client, repo, "branch")
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}
