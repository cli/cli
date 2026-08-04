package delete

import (
	"fmt"
	"net/http"
	"testing"

	cliapi "github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	ghapi "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutolinkDeleter_Delete(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name       string
		id         string
		stubStatus int
		stubResp   any

		expectErr      bool
		expectedErrMsg string
		expectedStatus int
	}{
		{
			name:       "204 successful delete",
			id:         "123",
			stubStatus: http.StatusNoContent,
		},
		{
			name:           "404 repo or autolink not found",
			id:             "123",
			stubStatus:     http.StatusNotFound,
			expectErr:      true,
			expectedErrMsg: "error deleting autolink: HTTP 404: Perhaps you are missing admin rights to the repository? (https://api.github.com/repos/OWNER/REPO/autolinks/123)",
		},
		{
			name: "500 unexpected error",
			id:   "123",
			stubResp: ghapi.HTTPError{
				Message: "arbitrary error",
			},
			stubStatus:     http.StatusInternalServerError,
			expectErr:      true,
			expectedErrMsg: "HTTP 500: arbitrary error (https://api.github.com/repos/OWNER/REPO/autolinks/123)",
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.REST(
					http.MethodDelete,
					fmt.Sprintf("repos/%s/%s/autolinks/%s", repo.RepoOwner(), repo.RepoName(), tt.id),
				),
				httpmock.StatusJSONResponse(tt.stubStatus, tt.stubResp),
			)
			defer reg.Verify(t)

			autolinkDeleter := &AutolinkDeleter{
				HTTPClient: &http.Client{Transport: reg},
			}

			err := autolinkDeleter.Delete(repo, tt.id)

			if tt.expectErr {
				require.EqualError(t, err, tt.expectedErrMsg)
				if tt.expectedStatus != 0 {
					var httpErr cliapi.HTTPError
					require.ErrorAs(t, err, &httpErr)
					assert.Equal(t, tt.expectedStatus, httpErr.StatusCode)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
