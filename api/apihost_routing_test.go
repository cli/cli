package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingTransport struct{ req *http.Request }

func (rt *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.req = r
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    r,
	}, nil
}

func TestAPIHostRouting(t *testing.T) {
	const overrideCfg = "hosts:\n  github.com:\n    oauth_token: canonical_tok\n    user: monalisa\n    api_host: gateway.internal\n"
	const plainCfg = "hosts:\n  github.com:\n    oauth_token: canonical_tok\n    user: monalisa\n"

	tests := []struct {
		name      string
		cfg       string
		newClient func(*http.Client, gh.Config) *Client
		path      string
		wantHost  string
		wantAuth  string
	}{
		{
			name: "carrier auto-routes the legacy constructor",
			cfg:  overrideCfg,
			// Exactly how command sites build a client today, unmodified.
			newClient: func(hc *http.Client, cfg gh.Config) *Client { return NewClientFromHTTP(WithConfig(hc, cfg)) },
			path:      "user",
			wantHost:  "gateway.internal",
			wantAuth:  "token canonical_tok",
		},
		{
			name:      "explicit config constructor routes",
			cfg:       overrideCfg,
			newClient: NewClientFromHTTPWithConfig,
			path:      "user",
			wantHost:  "gateway.internal",
			wantAuth:  "token canonical_tok",
		},
		{
			name:      "no override stays canonical",
			cfg:       plainCfg,
			newClient: func(hc *http.Client, cfg gh.Config) *Client { return NewClientFromHTTP(WithConfig(hc, cfg)) },
			path:      "user",
			wantHost:  "api.github.com",
			wantAuth:  "",
		},
		{
			name:      "server-provided canonical URL is re-mapped",
			cfg:       overrideCfg,
			newClient: func(hc *http.Client, cfg gh.Config) *Client { return NewClientFromHTTP(WithConfig(hc, cfg)) },
			path:      "https://api.github.com/repositories/1/issues?page=2",
			wantHost:  "gateway.internal",
			wantAuth:  "token canonical_tok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewFromString(tt.cfg)
			rt := &recordingTransport{}
			c := tt.newClient(&http.Client{Transport: rt}, cfg)

			var out map[string]interface{}
			_ = c.REST("github.com", "GET", tt.path, nil, &out)

			require.NotNil(t, rt.req)
			assert.Equal(t, tt.wantHost, rt.req.URL.Host)
			assert.Equal(t, tt.wantAuth, rt.req.Header.Get("Authorization"))
		})
	}
}
