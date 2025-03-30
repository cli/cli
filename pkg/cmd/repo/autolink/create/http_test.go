package create

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutolinkCreator_Create(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name         string
		req          AutolinkCreateRequest
		stubStatus   int
		stubRespJSON string

		expectedAutolink *shared.Autolink
		expectErr        bool
		expectedErrMsg   string
	}{
		{
			name: "201 successful creation",
			req: AutolinkCreateRequest{
				IsAlphanumeric: true,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "https://example.com/TICKET?query=<num>",
			},
			stubStatus: http.StatusCreated,
			stubRespJSON: `{
				"id":              1,
				"is_alphanumeric": true,
				"key_prefix":      "TICKET-",
				"url_template":    "https://example.com/TICKET?query=<num>"
			}`,
			expectedAutolink: &shared.Autolink{
				ID:             1,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "https://example.com/TICKET?query=<num>",
				IsAlphanumeric: true,
			},
		},
		{
			name: "422 URL template not valid URL",
			req: AutolinkCreateRequest{
				IsAlphanumeric: true,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "foo/<num>",
			},
			stubStatus: http.StatusUnprocessableEntity,
			stubRespJSON: `{
				"message": "Validation Failed",
				"errors": [
					{
						"resource": "KeyLink",
						"code": "custom",
						"field": "url_template",
						"message": "url_template must be an absolute URL"
					}
				],
				"documentation_url": "https://docs.github.com/rest/repos/autolinks#create-an-autolink-reference-for-a-repository",
				"status": "422"
				}`,
			expectErr: true,
			expectedErrMsg: heredoc.Doc(`
				HTTP 422: Validation Failed (https://api.github.com/repos/OWNER/REPO/autolinks)
				url_template must be an absolute URL`),
		},
		{
			name: "404 repo not found",
			req: AutolinkCreateRequest{
				IsAlphanumeric: true,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "https://example.com/TICKET?query=<num>",
			},
			stubStatus: http.StatusNotFound,
			stubRespJSON: `{
				"message": "Not Found",
				"documentation_url": "https://docs.github.com/rest/repos/autolinks#create-an-autolink-reference-for-a-repository",
				"status": "404"
			}`,
			expectErr:      true,
			expectedErrMsg: "HTTP 404: Must have admin rights to Repository. (https://api.github.com/repos/OWNER/REPO/autolinks)",
		},
		{
			name: "422 URL template missing <num>",
			req: AutolinkCreateRequest{
				IsAlphanumeric: true,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "https://example.com/TICKET",
			},
			stubStatus:   http.StatusUnprocessableEntity,
			stubRespJSON: `{"message":"Validation Failed","errors":[{"resource":"KeyLink","code":"custom","field":"url_template","message":"url_template is missing a <num> token"}],"documentation_url":"https://docs.github.com/rest/repos/autolinks#create-an-autolink-reference-for-a-repository","status":"422"}`,
			expectErr:    true,
			expectedErrMsg: heredoc.Doc(`
				HTTP 422: Validation Failed (https://api.github.com/repos/OWNER/REPO/autolinks)
				url_template is missing a <num> token`),
		},
		{
			name: "422 already exists",
			req: AutolinkCreateRequest{
				IsAlphanumeric: true,
				KeyPrefix:      "TICKET-",
				URLTemplate:    "https://example.com/TICKET?query=<num>",
			},
			stubStatus:   http.StatusUnprocessableEntity,
			stubRespJSON: `{"message":"Validation Failed","errors":[{"resource":"KeyLink","code":"already_exists","field":"key_prefix"}],"documentation_url":"https://docs.github.com/rest/repos/autolinks#create-an-autolink-reference-for-a-repository","status":"422"}`,
			expectErr:    true,
			expectedErrMsg: heredoc.Doc(`
				HTTP 422: Validation Failed (https://api.github.com/repos/OWNER/REPO/autolinks)
				KeyLink.key_prefix already exists`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.REST(
					http.MethodPost,
					fmt.Sprintf("repos/%s/%s/autolinks", repo.RepoOwner(), repo.RepoName())),
				httpmock.RESTPayload(tt.stubStatus, tt.stubRespJSON,
					func(payload map[string]interface{}) {
						require.Equal(t, map[string]interface{}{
							"is_alphanumeric": tt.req.IsAlphanumeric,
							"key_prefix":      tt.req.KeyPrefix,
							"url_template":    tt.req.URLTemplate,
						}, payload)
					},
				),
			)
			defer reg.Verify(t)

			autolinkCreator := &AutolinkCreator{
				HTTPClient: &http.Client{Transport: reg},
			}

			autolink, err := autolinkCreator.Create(repo, tt.req)

			if tt.expectErr {
				require.EqualError(t, err, tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedAutolink, autolink)
			}
		})
	}
}
