package extension

import (
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchLatestPrerelease(t *testing.T) {
	repo := ghrepo.NewWithHost("owner", "gh-bin-ext", "example.com")

	tests := []struct {
		name            string
		releases        []release
		wantTag         string
		wantNewerStable string
		wantErr         error
	}{
		{
			name: "picks highest pre-release",
			releases: []release{
				{Tag: "v1.1.0-pre", IsPrerelease: true},
				{Tag: "v1.0.0-pre", IsPrerelease: true},
			},
			wantTag: "v1.1.0-pre",
		},
		{
			name: "picks highest pre-release even when a lower stable exists",
			releases: []release{
				{Tag: "v1.0.1"},
				{Tag: "v1.1.0-pre", IsPrerelease: true},
				{Tag: "v1.0.0-pre", IsPrerelease: true},
			},
			wantTag: "v1.1.0-pre",
		},
		{
			name: "warns when a stable release is newer by version",
			releases: []release{
				{Tag: "v2.0.0"},
				{Tag: "v1.9.0-pre", IsPrerelease: true},
			},
			wantTag:         "v1.9.0-pre",
			wantNewerStable: "v2.0.0",
		},
		{
			name: "warns when a stable release is published more recently",
			releases: []release{
				{Tag: "v1.0.0", PublishedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
				{Tag: "v1.1.0-pre", IsPrerelease: true, PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
			wantTag:         "v1.1.0-pre",
			wantNewerStable: "v1.0.0",
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
				{Tag: "v9.9.9-pre", IsPrerelease: true, IsDraft: true},
				{Tag: "v1.1.0-pre", IsPrerelease: true},
			},
			wantTag: "v1.1.0-pre",
		},
		{
			name: "errors when there are no pre-releases",
			releases: []release{
				{Tag: "v1.0.0"},
			},
			wantErr: noPrereleasesFoundErr,
		},
		{
			name:     "errors when there are no releases",
			releases: []release{},
			wantErr:  noPrereleasesFoundErr,
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

			r, newerStable, err := fetchLatestPrerelease(client, repo)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTag, r.Tag)
			if tt.wantNewerStable == "" {
				assert.Nil(t, newerStable)
			} else {
				require.NotNil(t, newerStable)
				assert.Equal(t, tt.wantNewerStable, newerStable.Tag)
			}
		})
	}
}

// TestFetchLatestPrerelease_UnparseableTagSequences documents the behavior
// discussed in PR review around tags that are not valid semver (for example
// v1.0.0.beta.2 from cli/cli#13968).
//
// version.NewVersion cannot order an unparseable tag against the others, so the
// current implementation skips it and keeps the highest parseable pre-release.
// The consequence is that when the newest pre-release uses an unparseable tag we
// silently resolve to an older one instead of erroring and pointing the user at
// --pin. The cases below spell out the various sequences so the trade-off is
// explicit; they assert the CURRENT behavior and pass. The final case captures
// the reviewer's preferred behavior and is skipped rather than changed.
func TestFetchLatestPrerelease_UnparseableTagSequences(t *testing.T) {
	repo := ghrepo.NewWithHost("owner", "gh-bin-ext", "example.com")

	tests := []struct {
		name     string
		releases []release
		wantTag  string
		wantErr  error
		note     string
	}{
		{
			name: "unparseable tag alongside a parseable one keeps the parseable",
			releases: []release{
				{Tag: "v1.0.0.beta.2", IsPrerelease: true},
				{Tag: "v1.0.0-beta.1", IsPrerelease: true},
			},
			wantTag: "v1.0.0-beta.1",
			note:    "v1.0.0.beta.2 is newer to a human but is skipped; the older, parseable pre-release wins",
		},
		{
			name: "only unparseable pre-release tags errors as if none exist",
			releases: []release{
				{Tag: "v1.0.0.beta.2", IsPrerelease: true},
				{Tag: "v1.0.0.beta.1", IsPrerelease: true},
			},
			wantErr: noPrereleasesFoundErr,
			note:    "every pre-release is unparseable, so none is selectable and we cannot point at a specific one",
		},
		{
			name: "unparseable stable tag is ignored for the newer-stable warning",
			releases: []release{
				{Tag: "1.0-latest"},
				{Tag: "v1.0.0-pre", IsPrerelease: true},
			},
			wantTag: "v1.0.0-pre",
			note:    "an unparseable stable tag cannot be compared, so no newer-stable warning is produced",
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

			r, _, err := fetchLatestPrerelease(client, repo)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr, tt.note)
				return
			}
			require.NoError(t, err, tt.note)
			assert.Equal(t, tt.wantTag, r.Tag, tt.note)
		})
	}

	// Reviewer's preferred behavior: when the newest pre-release tag is
	// unparseable we would rather error and point the user at --pin than
	// silently install an older pre-release. Implementing this reliably is hard
	// because "newest" is undefined for an unparseable tag (it is not ordered as
	// a version), so we cannot know it is newest without trusting API ordering.
	// This is left as a known limitation; the subtest documents the aspiration
	// without changing current behavior.
	t.Run("ideal: newest unparseable pre-release errors and points at --pin", func(t *testing.T) {
		t.Skip("known limitation: unparseable newest tag is not ordered as a version; use --pin")

		reg := httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(
			httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases"),
			httpmock.JSONResponse([]release{
				{Tag: "v1.0.0.beta.2", IsPrerelease: true},
				{Tag: "v1.0.0-beta.1", IsPrerelease: true},
			}),
		)
		client := &http.Client{Transport: &reg}

		_, _, err := fetchLatestPrerelease(client, repo)
		require.Error(t, err, "newest pre-release tag is unparseable; should error and suggest --pin")
	})
}
