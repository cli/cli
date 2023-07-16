package api

import (
	"fmt"
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

type tinyConfig map[string]string

func (c tinyConfig) Token(host string) (string, string) {
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
