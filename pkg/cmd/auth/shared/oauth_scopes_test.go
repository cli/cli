package shared

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var disabledBearerConfig = gh.ConfigGetter(func(string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "disabled"}
})

var enabledBearerConfig = gh.ConfigGetter(func(string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "enabled"}
})

func Test_HasMinimumScopes(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantErr string
	}{
		{
			name:    "write:org satisfies read:org",
			header:  "repo, write:org",
			wantErr: "",
		},
		{
			name:    "insufficient scope",
			header:  "repo",
			wantErr: "missing required scope 'read:org'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakehttp := &httpmock.Registry{}
			defer fakehttp.Verify(t)

			var gotAuthorization string
			fakehttp.Register(httpmock.REST("GET", ""), func(req *http.Request) (*http.Response, error) {
				gotAuthorization = req.Header.Get("authorization")
				return &http.Response{
					Request:    req,
					StatusCode: 200,
					Body:       io.NopCloser(&bytes.Buffer{}),
					Header: map[string][]string{
						"X-Oauth-Scopes": {tt.header},
					},
				}, nil
			})

			client := http.Client{Transport: fakehttp}
			err := HasMinimumScopes(&client, "github.com", "ATOKEN", disabledBearerConfig)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, gotAuthorization, "token ATOKEN")
		})
	}
}

func Test_HasMinimumScopes_bearerAuth(t *testing.T) {
	fakehttp := &httpmock.Registry{}
	defer fakehttp.Verify(t)

	var gotAuthorization string
	fakehttp.Register(httpmock.REST("GET", ""), func(req *http.Request) (*http.Response, error) {
		gotAuthorization = req.Header.Get("authorization")
		return &http.Response{
			Request:    req,
			StatusCode: 200,
			Body:       io.NopCloser(&bytes.Buffer{}),
			Header: map[string][]string{
				"X-Oauth-Scopes": {"repo, read:org"},
			},
		}, nil
	})

	client := http.Client{Transport: fakehttp}
	err := HasMinimumScopes(&client, "github.com", "ATOKEN", enabledBearerConfig)
	require.NoError(t, err)
	assert.Equal(t, "Bearer ATOKEN", gotAuthorization)
}

func Test_HasMinimumScopes_bearerAuthFromEnv(t *testing.T) {
	fakehttp := &httpmock.Registry{}
	defer fakehttp.Verify(t)

	var gotAuthorization string
	fakehttp.Register(httpmock.REST("GET", ""), func(req *http.Request) (*http.Response, error) {
		gotAuthorization = req.Header.Get("authorization")
		return &http.Response{
			Request:    req,
			StatusCode: 200,
			Body:       io.NopCloser(&bytes.Buffer{}),
			Header: map[string][]string{
				"X-Oauth-Scopes": {"repo, read:org"},
			},
		}, nil
	})

	t.Setenv("GH_BEARER_AUTH", "1")
	client := http.Client{Transport: fakehttp}
	err := HasMinimumScopes(&client, "github.com", "ATOKEN", disabledBearerConfig)
	require.NoError(t, err)
	assert.Equal(t, "Bearer ATOKEN", gotAuthorization)
}

func Test_HeaderHasMinimumScopes(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantErr string
	}{
		{
			name:    "no scopes",
			header:  "",
			wantErr: "",
		},
		{
			name:    "default scopes",
			header:  "repo, read:org",
			wantErr: "",
		},
		{
			name:    "admin:org satisfies read:org",
			header:  "repo, admin:org",
			wantErr: "",
		},
		{
			name:    "write:org satisfies read:org",
			header:  "repo, write:org",
			wantErr: "",
		},
		{
			name:    "insufficient scope",
			header:  "repo",
			wantErr: "missing required scope 'read:org'",
		},
		{
			name:    "insufficient scopes",
			header:  "gist",
			wantErr: "missing required scopes 'repo', 'read:org'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			err := HeaderHasMinimumScopes(tt.header)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
