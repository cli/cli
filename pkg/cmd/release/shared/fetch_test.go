package shared

import (
	"context"
	"net/http"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRefSHA(t *testing.T) {
	tests := []struct {
		name            string
		tagName         string
		responseStatus  int
		responseBody    string
		responseMessage string
		expectedSHA     string
		errorMessage    string
	}{
		{
			name:           "match (200)",
			tagName:        "v1.2.3",
			responseStatus: 200,
			responseBody:   `{"object": {"sha": "1234567890abcdef1234567890abcdef12345678"}}`,
			expectedSHA:    "1234567890abcdef1234567890abcdef12345678",
		},
		{
			name:            "non-match (404)",
			tagName:         "v1.2.3",
			responseStatus:  404,
			responseMessage: `Not found`,
			errorMessage:    "release not found",
		},
		{
			name:            "server error (500)",
			tagName:         "v1.2.3",
			responseStatus:  500,
			responseMessage: `arbitrary error"`,
			errorMessage:    "HTTP 500: arbitrary error\" (https://api.github.com/repos/owner/repo/git/ref/tags%2Fv1.2.3)",
		},
		{
			name:           "malformed JSON with 200",
			tagName:        "v1.2.3",
			responseStatus: 200,
			responseBody:   `{"object": {"sha":`,
			errorMessage:   "failed to parse ref response: unexpected EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeHTTP := &httpmock.Registry{}
			defer fakeHTTP.Verify(t)

			repo, err := ghrepo.FromFullName("owner/repo")
			require.NoError(t, err)

			path := "repos/owner/repo/git/ref/tags%2F" + tt.tagName
			if tt.responseStatus == 404 || tt.responseStatus == 500 {
				fakeHTTP.Register(
					httpmock.REST("GET", path),
					httpmock.JSONErrorResponse(tt.responseStatus, api.HTTPError{
						StatusCode: tt.responseStatus,
						Message:    tt.responseMessage,
					}),
				)
			} else {
				fakeHTTP.Register(
					httpmock.REST("GET", path),
					httpmock.StatusStringResponse(tt.responseStatus, tt.responseBody),
				)
			}

			httpClient := &http.Client{Transport: fakeHTTP}
			ctx := context.Background()

			sha, err := FetchRefSHA(ctx, httpClient, repo, tt.tagName)

			if tt.errorMessage != "" {
				assert.Contains(t, err.Error(), tt.errorMessage)
				assert.Empty(t, sha)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedSHA, sha)
			}
		})
	}
}

func TestFetchLatestRelease(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		expectedTag    string
		expectedErr    error
		errorMessage   string
	}{
		{
			name:           "found (200)",
			responseStatus: 200,
			responseBody:   `{"tag_name": "v1.2.3"}`,
			expectedTag:    "v1.2.3",
		},
		{
			name:           "not found (404)",
			responseStatus: 404,
			expectedErr:    ErrReleaseNotFound,
		},
		{
			name:           "server error (500)",
			responseStatus: 500,
			errorMessage:   "HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeHTTP := &httpmock.Registry{}
			defer fakeHTTP.Verify(t)

			repo, err := ghrepo.FromFullName("owner/repo")
			require.NoError(t, err)

			if tt.responseStatus > 299 {
				fakeHTTP.Register(
					httpmock.REST("GET", "repos/owner/repo/releases/latest"),
					httpmock.JSONErrorResponse(tt.responseStatus, api.HTTPError{
						StatusCode: tt.responseStatus,
						Message:    "some error",
					}),
				)
			} else {
				fakeHTTP.Register(
					httpmock.REST("GET", "repos/owner/repo/releases/latest"),
					httpmock.StatusStringResponse(tt.responseStatus, tt.responseBody),
				)
			}

			release, err := FetchLatestRelease(context.Background(), &http.Client{Transport: fakeHTTP}, repo)

			switch {
			case tt.expectedErr != nil:
				require.ErrorIs(t, err, tt.expectedErr)
			case tt.errorMessage != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMessage)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.expectedTag, release.TagName)
			}
		})
	}
}

// TestFetchReleaseNotFoundWhenBothLookupsMiss pins the sentinel that FetchRelease reports when
// neither the published nor the draft lookup finds a release, since the published lookup now
// recognises a miss from an HTTPError rather than from the response status directly.
func TestFetchReleaseNotFoundWhenBothLookupsMiss(t *testing.T) {
	fakeHTTP := &httpmock.Registry{}

	repo, err := ghrepo.FromFullName("owner/repo")
	require.NoError(t, err)

	fakeHTTP.Register(
		httpmock.REST("GET", "repos/owner/repo/releases/tags/v9.9.9"),
		httpmock.JSONErrorResponse(404, api.HTTPError{StatusCode: 404, Message: "Not Found"}),
	)
	fakeHTTP.Register(
		httpmock.GraphQL(`query RepositoryReleaseByTag\b`),
		httpmock.StringResponse(`{"data": {"repository": {"release": null}}}`),
	)

	_, err = FetchRelease(context.Background(), &http.Client{Transport: fakeHTTP}, repo, "v9.9.9")
	require.ErrorIs(t, err, ErrReleaseNotFound)
}

func TestDigestAlgForRef(t *testing.T) {
	tests := []struct {
		name     string
		digest   string
		expected string
	}{
		{
			name:     "sha1 (40 hex chars)",
			digest:   "1234567890abcdef1234567890abcdef12345678",
			expected: "sha1",
		},
		{
			name:     "sha256 (64 hex chars)",
			digest:   "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: "sha256",
		},
		{
			name:     "empty string defaults to sha1",
			digest:   "",
			expected: "sha1",
		},
		{
			name:     "unexpected length defaults to sha1",
			digest:   "abc",
			expected: "sha1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DigestAlgForRef(tt.digest))
		})
	}
}
