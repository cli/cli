package capi

import (
	"context"
	"net/http"
	"net/url"
)

//go:generate moq -rm -out client_mock.go . CapiClient

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
	httpClient  *http.Client
	host        string
	capiBaseURL string
}

// NewCAPIClient creates a new CAPI client. Provide a token, the user's GitHub
// host, the resolved Copilot API URL, and an HTTP client which will be used as
// the base transport for CAPI requests.
//
// The provided HTTP client will be mutated for use with CAPI, so it should not
// be reused elsewhere.
func NewCAPIClient(httpClient *http.Client, token string, host string, capiBaseURL string) *CAPIClient {
	httpClient.Transport = newCAPITransport(token, capiBaseURL, httpClient.Transport)
	return &CAPIClient{
		httpClient:  httpClient,
		host:        host,
		capiBaseURL: capiBaseURL,
	}
}

// capiTransport adds the Copilot auth headers
type capiTransport struct {
	rp       http.RoundTripper
	token    string
	capiHost string
}

func newCAPITransport(token string, capiBaseURL string, rp http.RoundTripper) *capiTransport {
	capiHost := ""
	if u, err := url.Parse(capiBaseURL); err == nil {
		capiHost = u.Host
	}
	return &capiTransport{
		rp:       rp,
		token:    token,
		capiHost: capiHost,
	}
}

func (ct *capiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+ct.token)

	// Since this RoundTrip is reused for both Copilot API and
	// GitHub API requests, we conditionally add the integration
	// ID only when performing requests to the Copilot API.
	if req.URL.Host == ct.capiHost {
		req.Header.Add("Copilot-Integration-Id", "copilot-4-cli")

		// Ensure we are not using GitHub API versions while targeting CAPI.
		req.Header.Set("X-GitHub-Api-Version", "2026-01-09")
	}
	return ct.rp.RoundTrip(req)
}
