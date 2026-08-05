package githubrest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNextPage(t *testing.T) {
	tests := []struct {
		name       string
		linkHeader string
		wantNext   string
	}{
		{
			name:     "no Link header",
			wantNext: "",
		},
		{
			name:       "empty Link header",
			linkHeader: "",
			wantNext:   "",
		},
		{
			name:       "only a next relation",
			linkHeader: `<https://api.github.com/repositories/1/issues?page=2>; rel="next"`,
			wantNext:   "https://api.github.com/repositories/1/issues?page=2",
		},
		{
			name:       "multiple relations",
			linkHeader: `<https://api.github.com/repositories/1/issues?page=2>; rel="next", <https://api.github.com/repositories/1/issues?page=9>; rel="last"`,
			wantNext:   "https://api.github.com/repositories/1/issues?page=2",
		},
		{
			name:       "next is not the first relation",
			linkHeader: `<https://api.github.com/repositories/1/issues?page=1>; rel="prev", <https://api.github.com/repositories/1/issues?page=1>; rel="first", <https://api.github.com/repositories/1/issues?page=3>; rel="next"`,
			wantNext:   "https://api.github.com/repositories/1/issues?page=3",
		},
		{
			name:       "no next relation",
			linkHeader: `<https://api.github.com/repositories/1/issues?page=1>; rel="prev", <https://api.github.com/repositories/1/issues?page=1>; rel="first"`,
			wantNext:   "",
		},
		{
			// Cursor pagination supplies an opaque URL rather than a page
			// number, so the whole URL has to survive.
			name:       "cursor pagination",
			linkHeader: `<https://api.github.com/organizations/1/members?per_page=100&after=Y3Vyc29yOnYyOpHOAAcqDA%3D%3D>; rel="next"`,
			wantNext:   "https://api.github.com/organizations/1/members?per_page=100&after=Y3Vyc29yOnYyOpHOAAcqDA%3D%3D",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.linkHeader != "" {
				header.Set("Link", tt.linkHeader)
			}
			resp := &Response{Response: &http.Response{Header: header}}

			assert.Equal(t, tt.wantNext, resp.NextPage())
		})
	}
}
