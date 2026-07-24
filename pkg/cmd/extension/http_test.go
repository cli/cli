package extension

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchLatestPrerelease(t *testing.T) {
	repo := ghrepo.NewWithHost("owner", "gh-bin-ext", "example.com")

	tests := []struct {
		name     string
		releases []release
		wantTag  string
		wantErr  bool
	}{
		{
			name: "picks highest version including pre-releases",
			releases: []release{
				{Tag: "v1.0.1"},
				{Tag: "v1.1.0-pre", IsPrerelease: true},
				{Tag: "v1.0.0-pre", IsPrerelease: true},
			},
			wantTag: "v1.1.0-pre",
		},
		{
			name: "stable release wins when newer than any pre-release",
			releases: []release{
				{Tag: "v2.0.0"},
				{Tag: "v1.9.0-pre", IsPrerelease: true},
			},
			wantTag: "v2.0.0",
		},
		{
			name: "ignores list order in favor of version order",
			releases: []release{
				{Tag: "v1.0.0-pre", IsPrerelease: true},
				{Tag: "v3.0.0-pre", IsPrerelease: true},
				{Tag: "v2.0.0-pre", IsPrerelease: true},
			},
			wantTag: "v3.0.0-pre",
		},
		{
			name: "skips drafts",
			releases: []release{
				{Tag: "v9.9.9", IsDraft: true},
				{Tag: "v1.1.0-pre", IsPrerelease: true},
			},
			wantTag: "v1.1.0-pre",
		},
		{
			name: "falls back to first non-draft when versions are unparseable",
			releases: []release{
				{Tag: "not-a-version"},
				{Tag: "also-bad"},
			},
			wantTag: "not-a-version",
		},
		{
			name:     "errors when there are no releases",
			releases: []release{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases"),
				httpmock.JSONResponse(tt.releases),
			)
			client := &http.Client{Transport: &reg}

			r, err := fetchLatestPrerelease(client, repo)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTag, r.Tag)
		})
	}
}
