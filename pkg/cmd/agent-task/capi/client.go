package capi

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/internal/gh"
)

//go:generate moq -rm -out client_mock.go . CapiClient

const baseCAPIURL = "https://api.githubcopilot.com"
const capiHost = "api.githubcopilot.com"

// CapiClient defines the methods used by the caller. Implementations
// may be replaced with test doubles in unit tests.
type CapiClient interface {
	ListLatestSessionsForViewer(ctx context.Context, limit int) ([]*Session, error)
	CreateJob(ctx context.Context, owner, repo, problemStatement, baseBranch string, customAgent string) (*Job, error)
	GetJob(ctx context.Context, owner, repo, jobID string) (*Job, error)
	GetSession(ctx context.Context, id string) (*Session, error)
	GetSessionLogs(ctx context.Context, id string) ([]byte, error)
	ListSessionsByResourceID(ctx context.Context, resourceType string, resourceID int64, limit int) ([]*Session, error)
	GetPullRequestDatabaseID(ctx context.Context, hostname string, owner string, repo string, number int) (int64, string, error)
}

// CAPIClient is a client for interacting with the Copilot API
type CAPIClient struct {
	httpClient *http.Client
	authCfg    gh.AuthConfig
}

// NewCAPIClient creates a new CAPI client. Provide a token and an HTTP client which
// will be used as the base transport for CAPI requests.
//
// The provided HTTP client will be mutated for use with CAPI, so it should not
// be reused elsewhere.
func NewCAPIClient(httpClient *http.Client, authCfg gh.AuthConfig) *CAPIClient {
	host, _ := authCfg.DefaultHost()
	token, _ := authCfg.ActiveToken(host)

	httpClient.Transport = newCAPITransport(token, httpClient.Transport)
	return &CAPIClient{
		httpClient: httpClient,
		authCfg:    authCfg,
	}
}

// capiTransport adds the Copilot auth headers
type capiTransport struct {
	rp    http.RoundTripper
	token string
}

func newCAPITransport(token string, rp http.RoundTripper) *capiTransport {
	return &capiTransport{
		rp:    rp,
		token: token,
	}
}

func (ct *capiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+ct.token)

	// Since this RoundTrip is reused for both Copilot API and
	// GitHub API requests, we conditionally add the integration
	// ID only when performing requests to the Copilot API.
	if req.URL.Host == capiHost {
		req.Header.Add("Copilot-Integration-Id", "copilot-4-cli")
	}
	return ct.rp.RoundTrip(req)
}
