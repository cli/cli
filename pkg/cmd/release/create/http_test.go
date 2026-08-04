package create

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenHasWorkflowScope(t *testing.T) {
	tests := []struct {
		name   string
		scopes string
		want   bool
	}{
		{
			// The API separates scopes with a comma and a space, and lists them
			// in sorted order, so workflow is almost never the first element.
			name:   "realistically spaced scopes including workflow",
			scopes: "gist, project, read:org, repo, user, workflow",
			want:   true,
		},
		{
			name:   "spaced scopes without workflow",
			scopes: "repo, read:org",
			want:   false,
		},
		{
			name:   "workflow first",
			scopes: "workflow, repo",
			want:   true,
		},
		{
			name:   "only workflow",
			scopes: "workflow",
			want:   true,
		},
		{
			name:   "unspaced scopes including workflow",
			scopes: "repo,read:org,workflow",
			want:   true,
		},
		{
			// An absent header means this is not an OAuth token, so we can't
			// know its scopes and must assume workflow is present.
			name:   "no scopes header",
			scopes: "",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.scopes != "" {
				resp.Header.Set("X-Oauth-Scopes", tt.scopes)
			}

			assert.Equal(t, tt.want, tokenHasWorkflowScope(resp))
		})
	}
}
