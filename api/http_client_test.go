package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	type args struct {
		config         tokenGetter
		appVersion     string
		logVerboseHTTP bool
	}
	tests := []struct {
		name       string
		args       args
		host       string
		wantHeader map[string]string
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
			wantHeader: map[string]string{
				"authorization": "token MYTOKEN",
				"user-agent":    "GitHub CLI v1.2.3",
				"accept":        "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
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
			wantHeader: map[string]string{
				"authorization": "token GHETOKEN",
				"user-agent":    "GitHub CLI v1.2.3",
				"accept":        "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
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
			wantHeader: map[string]string{
				"authorization": "",
				"user-agent":    "GitHub CLI v1.2.3",
				"accept":        "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
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
			wantHeader: map[string]string{
				"authorization": "",
				"user-agent":    "GitHub CLI v1.2.3",
				"accept":        "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
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
			wantHeader: map[string]string{
				"authorization": "token MYTOKEN",
				"user-agent":    "GitHub CLI v1.2.3",
				"accept":        "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
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

				< HTTP/1.1 204 No Content
				< Date: <time>

				* Request took <duration>
			`),
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
				AppVersion:     tt.args.appVersion,
				Config:         tt.args.config,
				Log:            ios.ErrOut,
				LogVerboseHTTP: tt.args.logVerboseHTTP,
			})
			require.NoError(t, err)

			req, err := http.NewRequest("GET", ts.URL, nil)
			req.Header.Set("time-zone", "Europe/Amsterdam")
			req.Host = tt.host
			require.NoError(t, err)

			res, err := client.Do(req)

			require.NoError(t, err)

			for name, value := range tt.wantHeader {
				assert.Equal(t, value, gotReq.Header.Get(name), name)
			}

			assert.Equal(t, 204, res.StatusCode)
			assert.Equal(t, tt.wantStderr, normalizeVerboseLog(stderr.String()))
		})
	}
}

func TestHTTPClientRedirectAuthenticationHeaderHandling(t *testing.T) {
	var request *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var redirectRequest *http.Request
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectRequest = r
		http.Redirect(w, r, server.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	client, err := NewHTTPClient(HTTPClientOptions{
		Config: tinyConfig{
			fmt.Sprintf("%s:oauth_token", strings.TrimPrefix(redirectServer.URL, "http://")): "REDIRECT-TOKEN",
			fmt.Sprintf("%s:oauth_token", strings.TrimPrefix(server.URL, "http://")):         "TOKEN",
		},
	})
	require.NoError(t, err)

	req, err := http.NewRequest("GET", redirectServer.URL, nil)
	require.NoError(t, err)

	res, err := client.Do(req)
	require.NoError(t, err)

	assert.Equal(t, "token REDIRECT-TOKEN", redirectRequest.Header.Get(authorization))
	assert.Equal(t, "", request.Header.Get(authorization))
	assert.Equal(t, 204, res.StatusCode)
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

type tinyConfig map[string]string

func (c tinyConfig) ActiveToken(host string) (string, string) {
	return c[fmt.Sprintf("%s:%s", host, "oauth_token")], "oauth_token"
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
