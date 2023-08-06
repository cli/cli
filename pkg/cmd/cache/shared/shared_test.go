package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestGetCaches(t *testing.T) {
	tests := []struct {
		name       string
		opts       GetCachesOptions
		stubs      func(*httpmock.Registry)
		wantsCount int
	}{
		{
			name: "no caches",
			opts: GetCachesOptions{},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.StringResponse(`{"actions_caches": [], "total_count": 0}`),
				)
			},
			wantsCount: 0,
		},
		{
			name: "limits cache count",
			opts: GetCachesOptions{Limit: 1},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.StringResponse(`{"actions_caches": [{"id": 1}, {"id": 2}], "total_count": 2}`),
				)
			},
			wantsCount: 1,
		},
		{
			name: "negative limit returns all caches",
			opts: GetCachesOptions{Limit: -1},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.StringResponse(`{"actions_caches": [{"id": 1}, {"id": 2}], "total_count": 2}`),
				)
			},
			wantsCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			tt.stubs(reg)
			httpClient := &http.Client{Transport: reg}
			client := api.NewClientFromHTTP(httpClient)
			repo, err := ghrepo.FromFullName("OWNER/REPO")
			assert.NoError(t, err)
			result, err := GetCaches(client, repo, tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantsCount, len(result.ActionsCaches))
		})
	}
}
