package authflow

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_getViewer_leavesUserAgent(t *testing.T) {
	var receivedUA string
	var receivedAuth string

	plainClient := &http.Client{
		Transport: &roundTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			receivedUA = req.Header.Get("User-Agent")
			receivedAuth = req.Header.Get("Authorization")

			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewBufferString(`{"data":{"viewer":{"login":"monalisa"}}}`)),
				Request:    req,
			}, nil
		}},
	}

	login, err := getViewer(plainClient, "github.com", "test-token")
	require.NoError(t, err)
	assert.Equal(t, "monalisa", login)
	assert.Empty(t, receivedUA, "User-Agent header should be left unset so that downstream transports can set it")
	assert.Equal(t, "token test-token", receivedAuth)
}

type roundTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (t *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.roundTrip(req)
}

func Test_getCallbackURI(t *testing.T) {
	tests := []struct {
		name      string
		oauthHost string
		want      string
	}{
		{
			name:      "dotcom",
			oauthHost: "github.com",
			want:      "http://127.0.0.1/callback",
		},
		{
			name:      "ghes",
			oauthHost: "my.server.com",
			want:      "http://localhost/",
		},
		{
			name:      "ghec data residency (ghe.com)",
			oauthHost: "stampname.ghe.com",
			want:      "http://127.0.0.1/callback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getCallbackURI(tt.oauthHost))
		})
	}
}
