package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	type args struct {
		config     tokenGetter
		appVersion string
		setAccept  bool
	}
	tests := []struct {
		name       string
		args       args
		envDebug   string
		setGhDebug bool
		envGhDebug string
		host       string
		wantHeader map[string]string
		wantStderr string
	}{
		{
			name: "github.com with Accept header",
			args: args{
				config:     tinyConfig{"github.com:oauth_token": "MYTOKEN"},
				appVersion: "v1.2.3",
				setAccept:  true,
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
			name: "github.com no Accept header",
			args: args{
				config:     tinyConfig{"github.com:oauth_token": "MYTOKEN"},
				appVersion: "v1.2.3",
				setAccept:  false,
			},
			host: "github.com",
			wantHeader: map[string]string{
				"authorization": "token MYTOKEN",
				"user-agent":    "GitHub CLI v1.2.3",
				"accept":        "",
			},
			wantStderr: "",
		},
		{
			name: "github.com no authentication token",
			args: args{
				config:     tinyConfig{"example.com:oauth_token": "MYTOKEN"},
				appVersion: "v1.2.3",
				setAccept:  true,
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
			name: "github.com in verbose mode",
			args: args{
				config:     tinyConfig{"github.com:oauth_token": "MYTOKEN"},
				appVersion: "v1.2.3",
				setAccept:  true,
			},
			host:       "github.com",
			envDebug:   "api",
			setGhDebug: false,
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
		{
			name: "github.com in verbose mode",
			args: args{
				config:     tinyConfig{"github.com:oauth_token": "MYTOKEN"},
				appVersion: "v1.2.3",
				setAccept:  true,
			},
			host:       "github.com",
			envGhDebug: "api",
			setGhDebug: true,
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
		{
			name: "GHES Accept header",
			args: args{
				config:     tinyConfig{"example.com:oauth_token": "GHETOKEN"},
				appVersion: "v1.2.3",
				setAccept:  true,
			},
			host: "example.com",
			wantHeader: map[string]string{
				"authorization": "token GHETOKEN",
				"user-agent":    "GitHub CLI v1.2.3",
				"accept":        "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
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
			t.Setenv("DEBUG", tt.envDebug)
			if tt.setGhDebug {
				t.Setenv("GH_DEBUG", tt.envGhDebug)
			} else {
				os.Unsetenv("GH_DEBUG")
			}

			ios, _, _, stderr := iostreams.Test()
			client, err := NewHTTPClient(HTTPClientOptions{
				AppVersion:        tt.args.appVersion,
				Config:            tt.args.config,
				Log:               ios.ErrOut,
				SkipAcceptHeaders: !tt.args.setAccept,
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

func TestHTTPClient_SanitizeASCIIControlCharacters(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := Issue{
			Title: "\u001B[31mRed Title\u001B[0m",
			Body:  "1\u0001 2\u0002 3\u0003 4\u0004 5\u0005 6\u0006 7\u0007 8\u0008 9\t A\r\n B\u000b C\u000c D\r\n E\u000e F\u000f",
			Author: Author{
				ID:    "1",
				Name:  "10\u0010 11\u0011 12\u0012 13\u0013 14\u0014 15\u0015 16\u0016 17\u0017 18\u0018 19\u0019 1A\u001a 1B\u001b 1C\u001c 1D\u001d 1E\u001e 1F\u001f",
				Login: "monalisa",
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
	assert.Equal(t, "1^A 2^B 3^C 4^D 5^E 6^F 7^G 8^H 9\t A\r\n B^K C^L D\r\n E^N F^O", issue.Body)
	assert.Equal(t, "10^P 11^Q 12^R 13^S 14^T 15^U 16^V 17^W 18^X 19^Y 1A^Z 1B^[ 1C^\\ 1D^] 1E^^ 1F^_", issue.Author.Name)
	assert.Equal(t, "monalisa", issue.Author.Login)
	assert.Equal(t, "Escaped ^[ \\^[ \\^[ \\\\^[", issue.ActiveLockReason)
}

type tinyConfig map[string]string

func (c tinyConfig) AuthToken(host string) (string, string) {
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
