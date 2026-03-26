package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveCapiURL(t *testing.T) {
	tests := []struct {
		name    string
		resp    string
		wantURL string
		wantErr bool
	}{
		{
			name:    "returns resolved URL",
			resp:    `{"data":{"viewer":{"copilotEndpoints":{"api":"https://test-copilot-api.example.com"}}}}`,
			wantURL: "https://test-copilot-api.example.com",
		},
		{
			name:    "ghe.com tenant URL",
			resp:    `{"data":{"viewer":{"copilotEndpoints":{"api":"https://test-copilot-api.tenant.example.com"}}}}`,
			wantURL: "https://test-copilot-api.tenant.example.com",
		},
		{
			name:    "empty URL returns error",
			resp:    `{"data":{"viewer":{"copilotEndpoints":{"api":""}}}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			reg.Register(
				httpmock.GraphQL(`query CopilotEndpoints\b`),
				httpmock.StringResponse(tt.resp),
			)

			httpClient := &http.Client{Transport: reg}
			url, err := resolveCapiURL(httpClient, "github.com")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, url)
		})
	}
}

func TestCapiClientFuncResolvesURL(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query CopilotEndpoints\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"copilotEndpoints":{"api":"https://test-copilot-api.example.com"}}}}`),
	)

	f := &cmdutil.Factory{
		Config: func() (gh.Config, error) {
			return &ghmock.ConfigMock{
				AuthenticationFunc: func() gh.AuthConfig {
					c := &config.AuthConfig{}
					c.SetDefaultHost("github.com", "hosts")
					c.SetActiveToken("gho_TOKEN", "oauth_token")
					return c
				},
			}, nil
		},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
	}

	clientFunc := CapiClientFunc(f)
	client, err := clientFunc()
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify the GraphQL resolution was called
	require.Len(t, reg.Requests, 1)
}

func TestIsSession(t *testing.T) {
	assert.True(t, IsSessionID("00000000-0000-0000-0000-000000000000"))
	assert.True(t, IsSessionID("e2fa49d2-f164-4a56-ab99-498090b8fcdf"))
	assert.True(t, IsSessionID("E2FA49D2-F164-4A56-AB99-498090B8FCDF"))

	assert.False(t, IsSessionID(""))
	assert.False(t, IsSessionID(" "))
	assert.False(t, IsSessionID("\n"))
	assert.False(t, IsSessionID("not-a-uuid"))
	assert.False(t, IsSessionID("000000000000000000000000000000000000"))
	assert.False(t, IsSessionID("00000000-0000-0000-0000-000000000000-extra"))
}

func TestParsePullRequestAgentSessionURL(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		wantSessionID string
		wantErr       bool
	}{
		{
			name:          "valid",
			url:           "https://github.com/OWNER/REPO/pull/123/agent-sessions/e2fa49d2-f164-4a56-ab99-498090b8fcdf",
			wantSessionID: "e2fa49d2-f164-4a56-ab99-498090b8fcdf",
		},
		{
			name:    "invalid session id",
			url:     "https://github.com/OWNER/REPO/pull/123/agent-sessions/fff",
			wantErr: true,
		},
		{
			name:    "no session id, trailing slash",
			url:     "https://github.com/OWNER/REPO/pull/123/agent-sessions/",
			wantErr: true,
		},
		{
			name:    "no session id",
			url:     "https://github.com/OWNER/REPO/pull/123/agent-sessions",
			wantErr: true,
		},
		{
			name:    "invalid pr url",
			url:     "https://github.com/OWNER/REPO/issues/123",
			wantErr: true,
		},
		{
			name:    "empty",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID, err := ParseSessionIDFromURL(tt.url)

			if tt.wantErr {
				require.Error(t, err)
				assert.Zero(t, sessionID)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSessionID, sessionID)
		})
	}
}
