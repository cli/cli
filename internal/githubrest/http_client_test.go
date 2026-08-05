package githubrest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name       string
		auth       AuthStrategy
		wantHeader map[string]string
	}{
		{
			name: "with a token the client authenticates",
			auth: WithToken("MYTOKEN"),
			wantHeader: map[string]string{
				"Authorization": "token MYTOKEN",
			},
		},
		{
			// This is the property the whole duplicated constructor exists for.
			// Over api.NewHTTPClient's client, AddAuthTokenHeader would fill
			// this in from the host's active token.
			name: "without a token nothing authenticates the request",
			auth: WithoutToken(),
			wantHeader: map[string]string{
				"Authorization": "",
			},
		},
		{
			name: "go-gh's default headers survive Config being absent",
			auth: WithoutToken(),
			wantHeader: map[string]string{
				"Accept":               "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
				"Content-Type":         "application/json; charset=utf-8",
				"User-Agent":           "GitHub CLI v1.2.3",
				"X-GitHub-Api-Version": "2022-11-28",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotReq *http.Request
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotReq = r
				w.WriteHeader(http.StatusNoContent)
			}))
			defer ts.Close()

			httpClient, err := NewHTTPClient(HTTPClientOptions{AppVersion: "v1.2.3"})
			require.NoError(t, err)

			client, err := NewClient(ts.URL, httpClient, tt.auth)
			require.NoError(t, err)

			req, err := client.NewRequest(t.Context(), http.MethodGet, "user/repos", nil)
			require.NoError(t, err)

			_, err = client.Do(req, nil)
			require.NoError(t, err)

			for name, value := range tt.wantHeader {
				assert.Equal(t, value, gotReq.Header.Get(name), name)
			}
		})
	}
}
