package list

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/env/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentLister_List(t *testing.T) {
	tests := []struct {
		name        string
		repo        ghrepo.Interface
		resp        shared.EnvironmentPayload
		status      int
		expectedErr error
	}{
		{
			name:        "no environments",
			repo:        ghrepo.New("OWNER", "REPO"),
			resp:        shared.EnvironmentPayload{},
			status:      http.StatusOK,
			expectedErr: nil,
		},
		{
			name: "two environments",
			repo: ghrepo.New("OWNER", "REPO"),
			resp: shared.EnvironmentPayload{
				Environments: []shared.Environment{
					{
						Id:   1,
						Name: "dev",
					},
					{
						Id:   2,
						Name: "prod",
					},
				},
			},
			status:      http.StatusOK,
			expectedErr: nil,
		},
		{
			name:        "http 404 not found error",
			repo:        ghrepo.New("OWNER", "REPO"),
			resp:        shared.EnvironmentPayload{},
			status:      http.StatusNotFound,
			expectedErr: fmt.Errorf("HTTP 404 (https://api.github.com/repos/OWNER/REPO/environments?per_page=100)"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.REST(http.MethodGet, fmt.Sprintf("repos/%s/%s/environments", tt.repo.RepoOwner(), tt.repo.RepoName())),
				httpmock.StatusJSONResponse(tt.status, tt.resp),
			)
			defer reg.Verify(t)

			environmentLister := &EnvironmentLister{
				HTTPClient: &http.Client{Transport: reg},
			}
			environments, err := environmentLister.List(tt.repo, 100, false)
			if err != nil {
				require.Error(t, err)
				require.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.resp.Environments, environments)
			}
		})
	}
}
