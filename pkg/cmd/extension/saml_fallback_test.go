package extension

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWithSAMLFallback(t *testing.T) {
	samlResponse := func(req *http.Request) (*http.Response, error) {
		res := &http.Response{
			Request:    req,
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"message":"Resource protected by organization SAML enforcement"}`)),
		}
		res.Header.Set(ssoHeader, "required; url=https://github.com/orgs/o/sso")
		return res, nil
	}

	tests := []struct {
		name             string
		method           string
		body             io.Reader
		authedResponder  httpmock.Responder
		plainResponder   httpmock.Responder
		wantPlainCalls   int
		wantStatus       int
		wantBody         string
		wantAuthedHeader string
	}{
		{
			name:            "successful authenticated request is not retried",
			authedResponder: httpmock.StringResponse(`{"tag_name":"v1.0.0"}`),
			wantPlainCalls:  0,
			wantStatus:      http.StatusOK,
			wantBody:        `{"tag_name":"v1.0.0"}`,
		},
		{
			name:            "non-SAML failure is not retried",
			authedResponder: httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
			wantPlainCalls:  0,
			wantStatus:      http.StatusNotFound,
			wantBody:        `{"message":"Not Found"}`,
		},
		{
			name:            "403 without SSO header is not retried",
			authedResponder: httpmock.StatusStringResponse(http.StatusForbidden, `{"message":"Forbidden"}`),
			wantPlainCalls:  0,
			wantStatus:      http.StatusForbidden,
			wantBody:        `{"message":"Forbidden"}`,
		},
		{
			name:            "SAML 403 falls back to an unauthenticated request",
			authedResponder: samlResponse,
			plainResponder:  httpmock.StringResponse(`{"tag_name":"v1.0.0"}`),
			wantPlainCalls:  1,
			wantStatus:      http.StatusOK,
			wantBody:        `{"tag_name":"v1.0.0"}`,
		},
		{
			name:            "failed fallback returns the original authenticated response",
			authedResponder: samlResponse,
			plainResponder:  httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
			wantPlainCalls:  1,
			wantStatus:      http.StatusForbidden,
			wantBody:        `{"message":"Resource protected by organization SAML enforcement"}`,
		},
		{
			name:            "non-GET request is not retried",
			method:          http.MethodPost,
			authedResponder: samlResponse,
			wantPlainCalls:  0,
			wantStatus:      http.StatusForbidden,
			wantBody:        `{"message":"Resource protected by organization SAML enforcement"}`,
		},
		{
			name:            "GET with a body is not retried",
			body:            strings.NewReader(`{"query":"..."}`),
			authedResponder: samlResponse,
			wantPlainCalls:  0,
			wantStatus:      http.StatusForbidden,
			wantBody:        `{"message":"Resource protected by organization SAML enforcement"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authedReg := &httpmock.Registry{}
			defer authedReg.Verify(t)

			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			authedReg.Register(httpmock.REST(method, "repos/OWNER/REPO/releases/latest"), tt.authedResponder)

			plainReg := &httpmock.Registry{}
			if tt.plainResponder != nil {
				plainReg.Register(httpmock.REST(http.MethodGet, "repos/OWNER/REPO/releases/latest"), tt.plainResponder)
			}

			authedClient := &http.Client{Transport: setHeaderTransport{
				name: "Authorization", value: "token abc123", base: authedReg,
			}}
			plainClient := &http.Client{Transport: plainReg}

			req, err := http.NewRequest(method, "https://api.github.com/repos/OWNER/REPO/releases/latest", tt.body)
			require.NoError(t, err)

			res, err := WithSAMLFallback(authedClient, plainClient, nil).Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, res.StatusCode)
			assert.Equal(t, tt.wantBody, string(body))
			assert.Len(t, plainReg.Requests, tt.wantPlainCalls)
			for _, plainReq := range plainReg.Requests {
				assert.Empty(t, plainReq.Header.Get("Authorization"), "fallback request must not be authenticated")
			}
		})
	}
}

type setHeaderTransport struct {
	name  string
	value string
	base  http.RoundTripper
}

func (t setHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(t.name, t.value)
	return t.base.RoundTrip(req)
}

func TestWithSAMLFallback_redirectIsSuccess(t *testing.T) {
	authedReg := &httpmock.Registry{}
	defer authedReg.Verify(t)
	authedReg.Register(httpmock.REST("GET", "repos/OWNER/REPO/releases/assets/1"), func(req *http.Request) (*http.Response, error) {
		res := &http.Response{
			Request:    req,
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}
		res.Header.Set(ssoHeader, "required; url=https://github.com/orgs/o/sso")
		return res, nil
	})

	plainReg := &httpmock.Registry{}
	defer plainReg.Verify(t)
	plainReg.Register(httpmock.REST("GET", "repos/OWNER/REPO/releases/assets/1"), func(req *http.Request) (*http.Response, error) {
		res := &http.Response{
			Request:    req,
			StatusCode: http.StatusFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}
		res.Header.Set("Location", "https://release-assets.githubusercontent.com/asset")
		return res, nil
	})

	client := WithSAMLFallback(&http.Client{Transport: authedReg}, &http.Client{Transport: plainReg}, nil)

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/OWNER/REPO/releases/assets/1", nil)
	require.NoError(t, err)

	// RoundTrip rather than Do, so the redirect is observed instead of followed.
	res, err := client.Transport.RoundTrip(req)
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusFound, res.StatusCode)
	assert.Equal(t, "https://release-assets.githubusercontent.com/asset", res.Header.Get("Location"))
}

// Exercises the whole install path with the fallback wired the way
// pkg/cmd/factory does it, with every authenticated request SAML rejected.
// The outer http.Client follows the fallback's 302 back through this transport,
// so the second hop must not carry the token the authenticated chain set in place.
func TestWithSAMLFallback_followsRedirectWithoutAuth(t *testing.T) {
	var mu sync.Mutex
	var storageReqs []http.Header
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		storageReqs = append(storageReqs, r.Header.Clone())
		mu.Unlock()
		fmt.Fprint(w, "FAKE BINARY")
	}))
	defer storage.Close()

	const apiHost = "api.github.com"

	authed := funcRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != apiHost {
			return http.DefaultTransport.RoundTrip(req)
		}
		// Mimic the real chain, which attaches the token to req in place.
		req.Header.Set("Authorization", "token secret")
		res := &http.Response{
			Request:    req,
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}
		res.Header.Set(ssoHeader, "required; url=https://github.com/orgs/o/sso")
		return res, nil
	})

	plain := funcRoundTripper(func(req *http.Request) (*http.Response, error) {
		require.Empty(t, req.Header.Get("Authorization"), "fallback request must not be authenticated")
		res := &http.Response{
			Request:    req,
			StatusCode: http.StatusFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}
		res.Header.Set("Location", storage.URL+"/asset")
		return res, nil
	})

	var recovered int
	client := WithSAMLFallback(
		&http.Client{Transport: authed},
		&http.Client{Transport: plain},
		func() { recovered++ },
	)

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/OWNER/REPO/releases/assets/1", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/octet-stream")

	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "FAKE BINARY", string(body))
	assert.Equal(t, 1, recovered)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, storageReqs, 1)
	// Accept proves headers really do carry across the redirect, so the absent
	// Authorization below is meaningful rather than an artifact of nothing copying.
	assert.Equal(t, "application/octet-stream", storageReqs[0].Get("Accept"))
	assert.Empty(t, storageReqs[0].Get("Authorization"), "redirected request must not be authenticated")
}

func TestWithSAMLFallback_onRecoverNotCalledWithoutRecovery(t *testing.T) {
	authedReg := &httpmock.Registry{}
	defer authedReg.Verify(t)
	authedReg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`))

	var recovered int
	client := WithSAMLFallback(
		&http.Client{Transport: authedReg},
		&http.Client{Transport: &httpmock.Registry{}},
		func() { recovered++ },
	)

	res, err := client.Get("https://api.github.com/repos/OWNER/REPO/releases/latest")
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
	assert.Zero(t, recovered)
}

type funcRoundTripper func(*http.Request) (*http.Response, error)

func (f funcRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestManager_Install_binary_SAMLFallback(t *testing.T) {
	fakeExtensionName := "gh-bin-ext"
	repo := ghrepo.NewWithHost("owner", fakeExtensionName, "example.com")

	authedReg := &httpmock.Registry{}
	defer authedReg.Verify(t)
	samlRejected := func(req *http.Request) (*http.Response, error) {
		res := &http.Response{
			Request:    req,
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"message":"Resource protected by organization SAML enforcement"}`)),
		}
		res.Header.Set(ssoHeader, "required; url=https://example.com/orgs/owner/sso")
		return res, nil
	}
	for i := 0; i < 3; i++ {
		authedReg.Register(httpmock.MatchAny, samlRejected)
	}

	plainReg := &httpmock.Registry{}
	defer plainReg.Verify(t)
	latestRelease := release{
		Tag: "v1.0.1",
		Assets: []releaseAsset{
			{Name: "gh-bin-ext-windows-amd64.exe", APIURL: "https://example.com/release/cool"},
		},
	}
	plainReg.Register(httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"), httpmock.JSONResponse(latestRelease))
	plainReg.Register(httpmock.REST("GET", "api/v3/repos/owner/gh-bin-ext/releases/latest"), httpmock.JSONResponse(latestRelease))
	plainReg.Register(httpmock.REST("GET", "release/cool"), httpmock.StringResponse("FAKE BINARY"))

	ios, _, _, _ := iostreams.Test()
	dataDir := t.TempDir()

	client := WithSAMLFallback(&http.Client{Transport: authedReg}, &http.Client{Transport: plainReg}, nil)
	m := newTestManager(dataDir, t.TempDir(), client, nil, ios)

	require.NoError(t, m.Install(repo, ""))

	manifest, err := os.ReadFile(filepath.Join(dataDir, "extensions/gh-bin-ext", manifestName))
	require.NoError(t, err)

	var bm binManifest
	require.NoError(t, yaml.Unmarshal(manifest, &bm))
	assert.Equal(t, binManifest{
		Name:  fakeExtensionName,
		Owner: "owner",
		Host:  "example.com",
		Tag:   "v1.0.1",
		Path:  filepath.Join(dataDir, "extensions/gh-bin-ext/gh-bin-ext.exe"),
	}, bm)

	fakeBin, err := os.ReadFile(filepath.Join(dataDir, "extensions/gh-bin-ext/gh-bin-ext.exe"))
	require.NoError(t, err)
	assert.Equal(t, "FAKE BINARY", string(fakeBin))
}
