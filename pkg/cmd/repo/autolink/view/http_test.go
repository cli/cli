package view

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutolinkViewer_View(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name         string
		id           string
		stubStatus   int
		stubRespJSON string

		expectedAutolink *shared.Autolink
		expectErr        bool
		expectedErrMsg   string
	}{
		{
			name:       "200 successful alphanumeric view",
			id:         "123",
			stubStatus: 200,
			stubRespJSON: `{
				"id": 123,
				"key_prefix": "TICKET-",
				"url_template": "https://example.com/TICKET?query=<num>",
				"is_alphanumeric": true
			}`,
			expectedAutolink: &shared.Autolink{
				ID:             123,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "https://example.com/TICKET?query=<num>",
				IsAlphanumeric: true,
			},
		},
		{
			name:       "200 successful numeric view",
			id:         "123",
			stubStatus: 200,
			stubRespJSON: `{
				"id": 123,
				"key_prefix": "TICKET-",
				"url_template": "https://example.com/TICKET?query=<num>",
				"is_alphanumeric": false
			}`,
			expectedAutolink: &shared.Autolink{
				ID:             123,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "https://example.com/TICKET?query=<num>",
				IsAlphanumeric: false,
			},
		},
		{
			name:       "404 repo or autolink not found",
			id:         "123",
			stubStatus: 404,
			stubRespJSON: `{
				"message": "Not Found",
				"documentation_url": "https://docs.github.com/rest/repos/autolinks#get-an-autolink-reference-of-a-repository",
				"status": "404"
			}`,
			expectErr:      true,
			expectedErrMsg: "HTTP 404: Either no autolink with this ID exists for this repository or you are missing admin rights to the repository. (https://api.github.com/repos/OWNER/REPO/autolinks/123)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.REST(
					http.MethodGet,
					fmt.Sprintf("repos/%s/%s/autolinks/%s", repo.RepoOwner(), repo.RepoName(), tt.id),
				),
				httpmock.StatusStringResponse(tt.stubStatus, tt.stubRespJSON),
			)
			defer reg.Verify(t)

			autolinkCreator := &AutolinkViewer{
				HTTPClient: &http.Client{Transport: reg},
			}

			autolink, err := autolinkCreator.View(repo, tt.id)

			if tt.expectErr {
				require.EqualError(t, err, tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedAutolink, autolink)
			}
		})
	}
}
