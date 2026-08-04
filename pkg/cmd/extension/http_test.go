package extension

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extensionHTTPClient(t *testing.T, path string, status int, body string) *http.Client {
	t.Helper()

	reg := &httpmock.Registry{}
	t.Cleanup(func() {
		reg.Verify(t)
	})
	reg.Register(
		httpmock.REST(http.MethodGet, path),
		httpmock.StatusStringResponse(status, body),
	)
	return &http.Client{Transport: reg}
}

func requireExtensionHTTPError(t *testing.T, err error, status int) {
	t.Helper()

	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, status, httpErr.StatusCode)
	assert.Contains(t, err.Error(), fmt.Sprintf("HTTP %d", status))
}

func TestRepoExists(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	for name, body := range map[string]string{
		"JSON body":     `{}`,
		"empty body":    "",
		"non-JSON body": "repository",
	} {
		t.Run("success with "+name, func(t *testing.T) {
			client := extensionHTTPClient(t, "repos/OWNER/REPO", http.StatusOK, body)

			exists, err := repoExists(client, repo)

			require.NoError(t, err)
			assert.True(t, exists)
		})
	}

	t.Run("not found", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO", http.StatusNotFound, `{"message":"Not Found"}`)

		exists, err := repoExists(client, repo)

		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("server error", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO", http.StatusInternalServerError, `{"message":"Internal Server Error"}`)

		exists, err := repoExists(client, repo)

		assert.False(t, exists)
		requireExtensionHTTPError(t, err, http.StatusInternalServerError)
	})

	for _, status := range []int{http.StatusCreated, http.StatusNoContent} {
		t.Run("unexpected "+http.StatusText(status), func(t *testing.T) {
			client := extensionHTTPClient(t, "repos/OWNER/REPO", status, `{"message":"Unexpected status"}`)

			exists, err := repoExists(client, repo)

			assert.False(t, exists)
			requireExtensionHTTPError(t, err, status)
		})
	}
}

func TestHasScript(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	t.Run("success", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/contents/REPO", http.StatusOK, `{"type":"file"}`)

		hasScript, err := hasScript(client, repo)

		require.NoError(t, err)
		assert.True(t, hasScript)
	})

	// The contents endpoint returns an array when the requested path is a directory, and
	// objects with a non-file type for symlinks and submodules. None of these are treated as
	// a missing script, so they must not surface a decoding error.
	t.Run("success for non-file content", func(t *testing.T) {
		for name, body := range map[string]string{
			"directory listing": `[{"type":"file","name":"REPO"}]`,
			"directory":         `{"type":"dir"}`,
			"symlink":           `{"type":"symlink"}`,
			"submodule":         `{"type":"submodule"}`,
		} {
			t.Run(name, func(t *testing.T) {
				client := extensionHTTPClient(t, "repos/OWNER/REPO/contents/REPO", http.StatusOK, body)

				hasScript, err := hasScript(client, repo)

				require.NoError(t, err)
				assert.True(t, hasScript)
			})
		}
	})

	t.Run("not found", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/contents/REPO", http.StatusNotFound, `{"message":"Not Found"}`)

		hasScript, err := hasScript(client, repo)

		require.NoError(t, err)
		assert.False(t, hasScript)
	})

	t.Run("server error", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/contents/REPO", http.StatusInternalServerError, `{"message":"Internal Server Error"}`)

		hasScript, err := hasScript(client, repo)

		assert.False(t, hasScript)
		requireExtensionHTTPError(t, err, http.StatusInternalServerError)
	})
}

func TestFetchLatestRelease(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	t.Run("success", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/latest", http.StatusOK, `{"tag_name":"v1.2.3","assets":[{"name":"asset","url":"https://example.com/asset"}]}`)

		got, err := fetchLatestRelease(client, repo)

		require.NoError(t, err)
		assert.Equal(t, &release{
			Tag: "v1.2.3",
			Assets: []releaseAsset{{
				Name:   "asset",
				APIURL: "https://example.com/asset",
			}},
		}, got)
	})

	t.Run("not found", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/latest", http.StatusNotFound, `{"message":"Not Found"}`)

		got, err := fetchLatestRelease(client, repo)

		assert.Nil(t, got)
		require.Same(t, releaseNotFoundErr, err)
	})

	t.Run("server error", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/latest", http.StatusInternalServerError, `{"message":"Internal Server Error"}`)

		got, err := fetchLatestRelease(client, repo)

		assert.Nil(t, got)
		requireExtensionHTTPError(t, err, http.StatusInternalServerError)
	})

	for _, status := range []int{http.StatusNoContent, http.StatusResetContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/latest", status, "")

			got, err := fetchLatestRelease(client, repo)

			assert.Nil(t, got)
			require.EqualError(t, err, "unexpected end of JSON input")
		})
	}
}

func TestFetchReleaseFromTag(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	t.Run("success", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/tags/v1.2.3", http.StatusOK, `{"tag_name":"v1.2.3","assets":[{"name":"asset","url":"https://example.com/asset"}]}`)

		got, err := fetchReleaseFromTag(client, repo, "v1.2.3")

		require.NoError(t, err)
		assert.Equal(t, &release{
			Tag: "v1.2.3",
			Assets: []releaseAsset{{
				Name:   "asset",
				APIURL: "https://example.com/asset",
			}},
		}, got)
	})

	t.Run("not found", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/tags/v1.2.3", http.StatusNotFound, `{"message":"Not Found"}`)

		got, err := fetchReleaseFromTag(client, repo, "v1.2.3")

		assert.Nil(t, got)
		require.Same(t, releaseNotFoundErr, err)
	})

	t.Run("server error", func(t *testing.T) {
		client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/tags/v1.2.3", http.StatusInternalServerError, `{"message":"Internal Server Error"}`)

		got, err := fetchReleaseFromTag(client, repo, "v1.2.3")

		assert.Nil(t, got)
		requireExtensionHTTPError(t, err, http.StatusInternalServerError)
	})

	for _, status := range []int{http.StatusNoContent, http.StatusResetContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := extensionHTTPClient(t, "repos/OWNER/REPO/releases/tags/v1.2.3", status, "")

			got, err := fetchReleaseFromTag(client, repo, "v1.2.3")

			assert.Nil(t, got)
			require.EqualError(t, err, "unexpected end of JSON input")
		})
	}
}
