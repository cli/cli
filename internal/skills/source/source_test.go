package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRepoURL(t *testing.T) {
	assert.Equal(t, "https://github.com/monalisa/octocat-skills", BuildRepoURL("github.com", "monalisa", "octocat-skills"))
}

func TestParseMetadataRepo(t *testing.T) {
	tests := []struct {
		name      string
		meta      map[string]interface{}
		wantOwner string
		wantRepo  string
		wantHost  string
		wantFound bool
		wantErr   string
	}{
		{
			name: "parses repo url metadata",
			meta: map[string]interface{}{
				"github-repo": "https://github.com/monalisa/octocat-skills",
			},
			wantOwner: "monalisa",
			wantRepo:  "octocat-skills",
			wantHost:  SupportedHost,
			wantFound: true,
		},
		{
			name: "invalid repo url",
			meta: map[string]interface{}{
				"github-repo": "not a url",
			},
			wantFound: true,
			wantErr:   "invalid repository URL",
		},
		{
			name:      "missing repo metadata",
			meta:      map[string]interface{}{},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, found, err := ParseMetadataRepo(tt.meta)
			assert.Equal(t, tt.wantFound, found)
			if !tt.wantFound {
				require.NoError(t, err)
				assert.Nil(t, repo)
				return
			}
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, repo)
			assert.Equal(t, tt.wantOwner, repo.RepoOwner())
			assert.Equal(t, tt.wantRepo, repo.RepoName())
			assert.Equal(t, tt.wantHost, repo.RepoHost())
		})
	}
}

func TestValidateSupportedHost(t *testing.T) {
	require.NoError(t, ValidateSupportedHost("github.com"))
	require.NoError(t, ValidateSupportedHost("mycompany.ghe.com"), "GHEC data residency tenancy hosts should be accepted")
	require.ErrorContains(t, ValidateSupportedHost("acme.ghes.com"), "does not currently support GitHub Enterprise Server")
	require.ErrorContains(t, ValidateSupportedHost("github.localhost"), "unsupported host")
}
