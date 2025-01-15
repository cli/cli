package shared

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
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

func TestCache_ExportData(t *testing.T) {
	src := heredoc.Doc(
		`
		{
			"id": 505,
			"ref": "refs/heads/main",
			"key": "Linux-node-958aff96db2d75d67787d1e634ae70b659de937b",
			"version": "73885106f58cc52a7df9ec4d4a5622a5614813162cb516c759a30af6bf56e6f0",
			"last_accessed_at": "2019-01-24T22:45:36.000Z",
			"created_at": "2019-01-24T22:45:36.000Z",
			"size_in_bytes": 1024
		}
	`,
	)

	tests := []struct {
		name       string
		fields     []string
		inputJSON  string
		outputJSON string
	}{
		{
			name:      "basic",
			fields:    []string{"id", "key"},
			inputJSON: src,
			outputJSON: heredoc.Doc(
				`
				{
					"id": 505,
					"key": "Linux-node-958aff96db2d75d67787d1e634ae70b659de937b"
				}
			`,
			),
		},
		{
			name:      "full",
			fields:    []string{"id", "ref", "key", "version", "lastAccessedAt", "createdAt", "sizeInBytes"},
			inputJSON: src,
			outputJSON: heredoc.Doc(
				`
				{
					"createdAt": "2019-01-24T22:45:36Z",
					"id": 505,
					"key": "Linux-node-958aff96db2d75d67787d1e634ae70b659de937b",
					"lastAccessedAt": "2019-01-24T22:45:36Z",
					"ref": "refs/heads/main",
					"sizeInBytes": 1024,
					"version": "73885106f58cc52a7df9ec4d4a5622a5614813162cb516c759a30af6bf56e6f0"
				}
			`,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cache Cache
			dec := json.NewDecoder(strings.NewReader(tt.inputJSON))
			require.NoError(t, dec.Decode(&cache))

			exported := cache.ExportData(tt.fields)

			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			enc.SetIndent("", "\t")
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.outputJSON, buf.String())
		})
	}
}
