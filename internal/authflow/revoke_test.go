package authflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

type revokeResponseBody struct {
	io.Reader
	closed bool
}

func (b *revokeResponseBody) Close() error {
	b.closed = true
	return nil
}

func TestRevokeToken(t *testing.T) {
	tests := []struct {
		name         string
		host         string
		apiHost      string
		wantURL      string
		status       int
		transportErr error
		wantErr      string
	}{
		{
			name: "GitHub.com", host: "github.com", status: 204,
			wantURL: "https://api.github.com/applications/" + oauthClientID + "/token",
		},
		{
			name: "GHES", host: "enterprise.example.com", status: 204,
			wantURL: "https://enterprise.example.com/api/v3/applications/" + oauthClientID + "/token",
		},
		{
			name: "configured API gateway", host: "github.com", apiHost: "gateway.example.com", status: 204,
			wantURL: "https://gateway.example.com/applications/" + oauthClientID + "/token",
		},
		{
			name: "not found is not success", host: "github.com", status: 404,
			wantErr: "HTTP 404 (Not Found)",
		},
		{
			name: "server error omits response message", host: "github.com", status: 500,
			wantErr: "HTTP 500 (Internal Server Error)",
		},
		{
			name: "unexpected successful status", host: "github.com", status: 200,
			wantErr: "unexpected HTTP 200 for DELETE https://api.github.com/applications/" + oauthClientID + "/token",
		},
		{
			name: "does not follow redirects", host: "github.com", status: 302,
			wantErr: "HTTP 302 (Found)",
		},
		{
			name: "request timeout", host: "github.com", transportErr: context.DeadlineExceeded,
			wantErr: "request timed out",
		},
		{
			name: "request cancellation", host: "github.com", transportErr: context.Canceled,
			wantErr: "context canceled",
		},
		{
			name: "transport error redacts credentials", host: "github.com",
			transportErr: errors.New("connection failed with gho_test_secret and " + oauthClientSecret),
			wantErr:      "Delete \"https://api.github.com/applications/" + oauthClientID + "/token\": connection failed with [REDACTED] and [REDACTED]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := config.NewIsolatedTestConfig(t, "")
			if tt.apiHost != "" {
				cfg.Set(tt.host, "api_host", tt.apiHost)
			}
			const token = "gho_test_secret"
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			body := &revokeResponseBody{
				Reader: strings.NewReader(fmt.Sprintf(`{"message":"must not disclose %s or %s"}`, token, oauthClientSecret)),
			}
			wantURL := tt.wantURL
			if wantURL == "" {
				wantURL = "https://api.github.com/applications/" + oauthClientID + "/token"
			}
			expectedURL, err := url.Parse(wantURL)
			require.NoError(t, err)
			reg.Register(httpmock.REST("DELETE", strings.TrimPrefix(expectedURL.Path, "/")), func(req *http.Request) (*http.Response, error) {
				require.Equal(t, wantURL, req.URL.String())
				require.Empty(t, req.URL.RawQuery)
				clientID, secret, ok := req.BasicAuth()
				require.True(t, ok)
				require.Equal(t, oauthClientID, clientID)
				require.Equal(t, oauthClientSecret, secret)
				require.Equal(t, "application/vnd.github+json", req.Header.Get("Accept"))
				require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				payload, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"access_token":"gho_test_secret"}`, string(payload))
				deadline, ok := req.Context().Deadline()
				require.True(t, ok)
				require.Positive(t, time.Until(deadline))
				require.LessOrEqual(t, time.Until(deadline), 15*time.Second)
				if tt.transportErr != nil {
					return nil, tt.transportErr
				}
				return &http.Response{
					StatusCode: tt.status,
					Header: http.Header{
						"Content-Type": {"application/json"},
						"Location":     {"https://other.example.com/token"},
					},
					Body: body, Request: req,
				}, nil
			})
			err = RevokeToken(context.Background(), &http.Client{Transport: reg}, tt.host, token)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
				require.NotContains(t, err.Error(), token)
				require.NotContains(t, err.Error(), oauthClientSecret)
			}
			if tt.transportErr == nil {
				require.True(t, body.closed)
			}
		})
	}
}
