package api

import (
	"bytes"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/httpmock"
)

func TestBranchDeleteRemote(t *testing.T) {
	var tests = []struct {
		name        string
		code        int
		body        string
		expectError bool
	}{
		{name: "success", code: 204, body: "", expectError: false},
		{name: "error", code: 500, body: `{"message": "oh no"}`, expectError: true},
		{
			name:        "already_deleted",
			code:        422,
			body:        `{"message": "Reference does not exist"}`,
			expectError: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			client := NewClient(ReplaceTripper(http))

			http.StubResponse(tc.code, bytes.NewBufferString(tc.body))
			repo, _ := ghrepo.FromFullName("OWNER/REPO")
			err := BranchDeleteRemote(client, repo, "branch")
			if isError := err != nil; isError != tc.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}
