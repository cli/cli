package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	type args struct {
		config             config
		appVersion         string
		invokingAgent      string
		logVerboseHTTP     bool
		skipDefaultHeaders bool
	}
	tests := []struct {
		name       string
		args       args
		host       string
		wantHeader map[string][]string
		wantStderr string
	}{
		{
			name: "github.com",
			args: args{
				config:         tinyConfig{"github.com:oauth_token": "MYTOKEN"},
				appVersion:     "v1.2.3",
				logVerboseHTTP: false,
			},
			host: "github.com",
			wantHeader: map[string][]string{
				"authorization":        {"token MYTOKEN"},
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: "",
		},
		{
			name: "GHES",
			args: args{
				config:     tinyConfig{"example.com:oauth_token": "GHETOKEN"},
				appVersion: "v1.2.3",
			},
			host: "example.com",
			wantHeader: map[string][]string{
				"authorization":        {"token GHETOKEN"},
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: "",
		},
		{
			name: "api_host is sent the token of the host it stands in for",
			args: args{
				config: tinyConfig{
					"github.com:oauth_token":    "MYTOKEN",
					"api_host:gateway.internal": "github.com",
				},
				appVersion: "v1.2.3",
			},
			host: "gateway.internal",
			wantHeader: map[string][]string{
				"authorization":        {"token MYTOKEN"},
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: "",
		},
		{
			name: "a host with its own token is unaffected by an api_host mapping",
			args: args{
				config: tinyConfig{
					"github.com:oauth_token":       "MYTOKEN",
					"gateway.internal:oauth_token": "OWNTOKEN",
					"api_host:gateway.internal":    "github.com",
				},
				appVersion: "v1.2.3",
			},
			host: "gateway.internal",
			wantHeader: map[string][]string{
				"authorization":        {"token OWNTOKEN"},
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: "",
		},
		{
			name: "an unmapped host is still sent no token",
			args: args{
				config: tinyConfig{
					"github.com:oauth_token":    "MYTOKEN",
					"api_host:gateway.internal": "github.com",
				},
				appVersion: "v1.2.3",
			},
			host: "elsewhere.internal",
			wantHeader: map[string][]string{
				"authorization":        nil, // should not be set
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: "",
		},
		{
			name: "github.com no authentication token",
			args: args{
				config:         tinyConfig{"example.com:oauth_token": "MYTOKEN"},
				appVersion:     "v1.2.3",
				logVerboseHTTP: false,
			},
			host: "github.com",
			wantHeader: map[string][]string{
				"authorization":        nil, // should not be set
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: "",
		},
		{
			name: "GHES no authentication token",
			args: args{
				config:         tinyConfig{"github.com:oauth_token": "MYTOKEN"},
				appVersion:     "v1.2.3",
				logVerboseHTTP: false,
			},
			host: "example.com",
			wantHeader: map[string][]string{
				"authorization":        nil, // should not be set
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: "",
		},
		{
			name: "github.com in verbose mode",
			args: args{
				config:         tinyConfig{"github.com:oauth_token": "MYTOKEN"},
				appVersion:     "v1.2.3",
				logVerboseHTTP: true,
			},
			host: "github.com",
			wantHeader: map[string][]string{
				"authorization":        {"token MYTOKEN"},
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
				"accept":               {"application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview"},
			},
			wantStderr: heredoc.Doc(`
				* Request at <time>
				* Request to http://<host>:<port>
				> GET / HTTP/1.1
				> Host: github.com
				> Accept: application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview
				> Authorization: token ████████████████████
				> Content-Type: application/json; charset=utf-8
				> Time-Zone: <timezone>
				> User-Agent: GitHub CLI v1.2.3
				> X-Github-Api-Version: 2022-11-28

				< HTTP/1.1 204 No Content
				< Date: <time>

				* Request took <duration>
			`),
		},
		{
			name: "respect skip default headers option",
			args: args{
				appVersion:         "v1.2.3",
				logVerboseHTTP:     true,
				skipDefaultHeaders: true,
			},
			host: "github.com",
			wantHeader: map[string][]string{
				"accept":               nil,
				"authorization":        nil,
				"content-type":         nil,
				"user-agent":           {"GitHub CLI v1.2.3"},
				"x-github-api-version": {"2022-11-28"},
			},
			wantStderr: heredoc.Doc(`
				* Request at <time>
				* Request to http://<host>:<port>
				> GET / HTTP/1.1
				> Host: github.com
				> Time-Zone: <timezone>
				> User-Agent: GitHub CLI v1.2.3
				> X-Github-Api-Version: 2022-11-28

				< HTTP/1.1 204 No Content
				< Date: <time>

				* Request took <duration>
			`),
		},
		{
			name: "includes invoking agent in user-agent header",
			args: args{
				appVersion:    "v1.2.3",
				invokingAgent: "copilot-cli",
			},
			host: "github.com",
			wantHeader: map[string][]string{
				"user-agent": {"GitHub CLI v1.2.3 Agent/copilot-cli"},
			},
			wantStderr: "",
		},
	}

	var gotReq *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stderr := iostreams.Test()
			client, err := NewHTTPClient(HTTPClientOptions{
				AppVersion:         tt.args.appVersion,
				InvokingAgent:      tt.args.invokingAgent,
				Config:             tt.args.config,
				Log:                ios.ErrOut,
				LogVerboseHTTP:     tt.args.logVerboseHTTP,
				SkipDefaultHeaders: tt.args.skipDefaultHeaders,
			})
			require.NoError(t, err)

			req, err := http.NewRequest("GET", ts.URL, nil)
			req.Header.Set("time-zone", "Europe/Amsterdam")
			req.Host = tt.host
			require.NoError(t, err)

			res, err := client.Do(req)

			require.NoError(t, err)

			for name, value := range tt.wantHeader {
				assert.Equal(t, value, gotReq.Header.Values(name), name)
			}

			assert.Equal(t, 204, res.StatusCode)
			assert.Equal(t, tt.wantStderr, normalizeVerboseLog(stderr.String()))
		})
	}
}

func TestHTTPClientRedirectAuthenticationHeaderHandling(t *testing.T) {
	// Two servers stand in for two different hosts. A dial map lets the test
	// address them by hostname, so the auth layer compares real hostnames rather
	// than the ephemeral ports of localhost servers, which it no longer treats as
	// part of the host.
	var serverRequest *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverRequest = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var redirectRequest *http.Request
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectRequest = r
		http.Redirect(w, r, "http://canonical.example/", http.StatusFound)
	}))
	defer redirectServer.Close()

	hostToAddr := map[string]string{
		"other.example":     redirectServer.Listener.Addr().String(),
		"canonical.example": server.Listener.Addr().String(),
	}
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if host, _, err := net.SplitHostPort(addr); err == nil {
				if mapped, ok := hostToAddr[host]; ok {
					addr = mapped
				}
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	config := tinyConfig{
		"other.example:oauth_token":     "OTHER-TOKEN",
		"canonical.example:oauth_token": "CANONICAL-TOKEN",
	}
	client := &http.Client{Transport: AddAuthTokenHeader(transport, config)}

	req, err := http.NewRequest("GET", "http://other.example/", nil)
	require.NoError(t, err)

	res, err := client.Do(req)
	require.NoError(t, err)

	// The initial request is authenticated as its own host.
	assert.Equal(t, "token OTHER-TOKEN", redirectRequest.Header.Get(authorization))
	// Following the redirect crosses to a different host, so no token is attached,
	// even though one is configured for that host.
	assert.Equal(t, "", serverRequest.Header.Get(authorization))
	assert.Equal(t, 204, res.StatusCode)
}

// serverHostname returns the hostname of a test server URL, so config can be
// keyed by host, as production config is, rather than host:port.
func serverHostname(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u.Hostname()
}

func TestHTTPClientSanitizeJSONControlCharactersC0(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := Issue{
			Title: "\u001B[31mRed Title\u001B[0m",
			Body:  "1\u0001 2\u0002 3\u0003 4\u0004 5\u0005 6\u0006 7\u0007 8\u0008 9\t A\r\n B\u000b C\u000c D\r\n E\u000e F\u000f",
			Author: Author{
				ID:    "1",
				Name:  "10\u0010 11\u0011 12\u0012 13\u0013 14\u0014 15\u0015 16\u0016 17\u0017 18\u0018 19\u0019 1A\u001a 1B\u001b 1C\u001c 1D\u001d 1E\u001e 1F\u001f",
				Login: "monalisa \\u00\u001b",
			},
			ActiveLockReason: "Escaped \u001B \\u001B \\\u001B \\\\u001B",
		}
		responseData, _ := json.Marshal(issue)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, string(responseData))
	}))
	defer ts.Close()

	client, err := NewHTTPClient(HTTPClientOptions{})
	require.NoError(t, err)
	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)
	var issue Issue
	err = json.Unmarshal(body, &issue)
	require.NoError(t, err)
	assert.Equal(t, "^[[31mRed Title^[[0m", issue.Title)
	assert.Equal(t, "1^A 2^B 3^C 4^D 5^E 6^F 7^G 8\b 9\t A\r\n B\v C\f D\r\n E^N F^O", issue.Body)
	assert.Equal(t, "10^P 11^Q 12^R 13^S 14^T 15^U 16^V 17^W 18^X 19^Y 1A^Z 1B^[ 1C^\\ 1D^] 1E^^ 1F^_", issue.Author.Name)
	assert.Equal(t, "monalisa \\u00^[", issue.Author.Login)
	assert.Equal(t, "Escaped ^[ \\^[ \\^[ \\\\^[", issue.ActiveLockReason)
}

func TestHTTPClientSanitizeControlCharactersC1(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := Issue{
			Title: "\xC2\x9B[31mRed Title\xC2\x9B[0m",
			Body:  "80\xC2\x80 81\xC2\x81 82\xC2\x82 83\xC2\x83 84\xC2\x84 85\xC2\x85 86\xC2\x86 87\xC2\x87 88\xC2\x88 89\xC2\x89 8A\xC2\x8A 8B\xC2\x8B 8C\xC2\x8C 8D\xC2\x8D 8E\xC2\x8E 8F\xC2\x8F",
			Author: Author{
				ID:    "1",
				Name:  "90\xC2\x90 91\xC2\x91 92\xC2\x92 93\xC2\x93 94\xC2\x94 95\xC2\x95 96\xC2\x96 97\xC2\x97 98\xC2\x98 99\xC2\x99 9A\xC2\x9A 9B\xC2\x9B 9C\xC2\x9C 9D\xC2\x9D 9E\xC2\x9E 9F\xC2\x9F",
				Login: "monalisa\xC2\xA1",
			},
		}
		responseData, _ := json.Marshal(issue)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprint(w, string(responseData))
	}))
	defer ts.Close()

	client, err := NewHTTPClient(HTTPClientOptions{})
	require.NoError(t, err)
	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)
	var issue Issue
	err = json.Unmarshal(body, &issue)
	require.NoError(t, err)
	assert.Equal(t, "^[[31mRed Title^[[0m", issue.Title)
	assert.Equal(t, "80^@ 81^A 82^B 83^C 84^D 85^E 86^F 87^G 88^H 89^I 8A^J 8B^K 8C^L 8D^M 8E^N 8F^O", issue.Body)
	assert.Equal(t, "90^P 91^Q 92^R 93^S 94^T 95^U 96^V 97^W 98^X 99^Y 9A^Z 9B^[ 9C^\\ 9D^] 9E^^ 9F^_", issue.Author.Name)
	assert.Equal(t, "monalisa¡", issue.Author.Login)
}

func TestNewHTTPClientTelemetryDisabler(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	tests := []struct {
		name         string
		host         string
		wantDisabled bool
	}{
		{
			name:         "enterprise host triggers disable",
			host:         "ghes.example.com",
			wantDisabled: true,
		},
		{
			name:         "github.com does not trigger disable",
			host:         "github.com",
			wantDisabled: false,
		},
		{
			name:         "tenancy host does not trigger disable",
			host:         "my-company.ghe.com",
			wantDisabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disabler := &fakeTelemetryDisabler{}
			client, err := NewHTTPClient(HTTPClientOptions{
				TelemetryDisabler: disabler,
			})
			require.NoError(t, err)

			req, err := http.NewRequest("GET", ts.URL, nil)
			require.NoError(t, err)
			req.Host = tt.host

			res, err := client.Do(req)
			require.NoError(t, err)
			assert.Equal(t, 204, res.StatusCode)
			assert.Equal(t, tt.wantDisabled, disabler.disabled, "Disable() called")
		})
	}
}

func TestNewHTTPClientWithoutTelemetryDisabler(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client, err := NewHTTPClient(HTTPClientOptions{})
	require.NoError(t, err)

	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)
	req.Host = "ghes.example.com"

	res, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)
}

func TestNewHTTPClientRecordsRequestIDs(t *testing.T) {
	recorder := &fakeRequestIDRecorder{}
	responses := []*http.Response{
		{
			StatusCode: http.StatusFound,
			Header: http.Header{
				"Location":            []string{"https://api.github.com/final"},
				"X-Github-Request-Id": []string{"REQUEST-1"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		},
		{
			StatusCode: http.StatusNoContent,
			Header: http.Header{
				"X-Github-Request-Id": []string{"REQUEST-2"},
			},
			Body: io.NopCloser(strings.NewReader("")),
		},
	}
	transport := &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		res := responses[0]
		responses = responses[1:]
		res.Request = req
		return res, nil
	}}
	client, err := NewHTTPClient(HTTPClientOptions{
		RequestIDRecorder: recorder,
		Transport:         transport,
	})
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "https://api.github.com/start", nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	res.Body.Close()

	assert.Equal(t, http.StatusNoContent, res.StatusCode)
	assert.Equal(t, []string{"REQUEST-1", "REQUEST-2"}, recorder.requestIDs)
}

func TestNewHTTPClientDoesNotRecordMissingRequestID(t *testing.T) {
	recorder := &fakeRequestIDRecorder{}
	client, err := NewHTTPClient(HTTPClientOptions{
		RequestIDRecorder: recorder,
		Transport: &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     http.Header{"X-Github-Request-Id": []string{"  "}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}},
	})
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "https://api.github.com/", nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	res.Body.Close()

	assert.Empty(t, recorder.requestIDs)
}

func TestNewHTTPClientDoesNotReplayCachedRequestIDs(t *testing.T) {
	recorder := &fakeRequestIDRecorder{}
	requestCount := 0
	client, err := NewHTTPClient(HTTPClientOptions{
		CacheDir:          t.TempDir(),
		CacheTTL:          time.Hour,
		EnableCache:       true,
		RequestIDRecorder: recorder,
		Transport: &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			requestCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"X-Github-Request-Id": []string{fmt.Sprintf("REQUEST-%d", requestCount)}},
				Body:       io.NopCloser(strings.NewReader("{}")),
				Request:    req,
			}, nil
		}},
	})
	require.NoError(t, err)

	for range 2 {
		req, err := http.NewRequest("GET", "https://api.github.com/repos/cli/cli", nil)
		require.NoError(t, err)
		res, err := client.Do(req)
		require.NoError(t, err)
		_, err = io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
	}

	assert.Equal(t, 1, requestCount)
	assert.Equal(t, []string{"REQUEST-1"}, recorder.requestIDs)
}

func TestNewHTTPClientDropsTelemetryForEnterpriseRequests(t *testing.T) {
	var payload telemetry.SendTelemetryPayload
	recorder := telemetry.NewService(func(p telemetry.SendTelemetryPayload) {
		payload = p
	})
	client, err := NewHTTPClient(HTTPClientOptions{
		RequestIDRecorder: recorder,
		TelemetryDisabler: recorder,
		Transport: &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     http.Header{"X-Github-Request-Id": []string{"ENTERPRISE-REQUEST"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}},
	})
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "https://ghes.example.com/api/v3/user", nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	res.Body.Close()
	recorder.Flush()

	assert.Empty(t, payload.Events)
}

func TestNewExternalHTTPClient(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "third-party host",
			url:  "https://example.com/path",
		},
		{
			// Even when talking to GitHub, the external client must not set
			// authorization or any GitHub-specific headers.
			name: "github.com host",
			url:  "https://api.github.com/repos/cli/cli",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotReq *http.Request
			transport := &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
				gotReq = req
				return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
			}}

			client, err := NewExternalHTTPClient(ExternalHTTPClientOptions{
				AppVersion: "v1.2.3",
				Transport:  transport,
			})
			require.NoError(t, err)

			req, err := http.NewRequest("GET", tt.url, nil)
			require.NoError(t, err)

			res, err := client.Do(req)
			require.NoError(t, err)
			assert.Equal(t, 204, res.StatusCode)

			// No headers should be set by default, except for User-Agent which should include the app version.
			assert.Equal(t, []string{"GitHub CLI v1.2.3"}, gotReq.Header.Values("user-agent"))
			assert.Empty(t, gotReq.Header.Values("authorization"))
			assert.Empty(t, gotReq.Header.Values("x-github-api-version"))
			assert.Empty(t, gotReq.Header.Values("accept"))
			assert.Empty(t, gotReq.Header.Values("content-type"))
			assert.Empty(t, gotReq.Header.Values("time-zone"))
		})
	}
}

type fakeTelemetryDisabler struct {
	disabled bool
}

func (f *fakeTelemetryDisabler) Disable() {
	f.disabled = true
}

type fakeRequestIDRecorder struct {
	requestIDs []string
	disabled   bool
}

func (f *fakeRequestIDRecorder) RecordRequestID(requestID string) {
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		f.requestIDs = append(f.requestIDs, requestID)
	}
}

func (f *fakeRequestIDRecorder) Disable() {
	f.disabled = true
}

type tinyConfig map[string]string

func (c tinyConfig) ActiveToken(host string) (string, string) {
	return c[fmt.Sprintf("%s:%s", host, "oauth_token")], "oauth_token"
}

// HostForAPIHost resolves via an "api_host:<apiHost>" key holding the host that
// configured it, mirroring the reverse lookup the real config performs.
func (c tinyConfig) HostForAPIHost(apiHost string) (string, bool) {
	host, ok := c[fmt.Sprintf("%s:%s", "api_host", apiHost)]
	return host, ok
}

var requestAtRE = regexp.MustCompile(`(?m)^\* Request at .+`)
var dateRE = regexp.MustCompile(`(?m)^< Date: .+`)
var hostWithPortRE = regexp.MustCompile(`127\.0\.0\.1:\d+`)
var durationRE = regexp.MustCompile(`(?m)^\* Request took .+`)
var timezoneRE = regexp.MustCompile(`(?m)^> Time-Zone: .+`)

func normalizeVerboseLog(t string) string {
	t = requestAtRE.ReplaceAllString(t, "* Request at <time>")
	t = hostWithPortRE.ReplaceAllString(t, "<host>:<port>")
	t = dateRE.ReplaceAllString(t, "< Date: <time>")
	t = durationRE.ReplaceAllString(t, "* Request took <duration>")
	t = timezoneRE.ReplaceAllString(t, "> Time-Zone: <timezone>")
	return t
}
