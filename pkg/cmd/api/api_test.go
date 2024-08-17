package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdApi(t *testing.T) {
	f := &cmdutil.Factory{}

	tests := []struct {
		name     string
		cli      string
		wants    ApiOptions
		wantsErr bool
	}{
		{
			name: "no flags",
			cli:  "graphql",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "graphql",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "override method",
			cli:  "repos/octocat/Spoon-Knife -XDELETE",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "DELETE",
				RequestMethodPassed: true,
				RequestPath:         "repos/octocat/Spoon-Knife",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "with fields",
			cli:  "graphql -f query=QUERY -F body=@file.txt",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "graphql",
				RequestInputFile:    "",
				RawFields:           []string{"query=QUERY"},
				MagicFields:         []string{"body=@file.txt"},
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "with headers",
			cli:  "user -H 'accept: text/plain' -i",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "user",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string{"accept: text/plain"},
				ShowResponseHeaders: true,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "with pagination",
			cli:  "repos/OWNER/REPO/issues --paginate",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "repos/OWNER/REPO/issues",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            true,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "with silenced output",
			cli:  "repos/OWNER/REPO/issues --silent",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "repos/OWNER/REPO/issues",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              true,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name:     "POST pagination",
			cli:      "-XPOST repos/OWNER/REPO/issues --paginate",
			wantsErr: true,
		},
		{
			name: "GraphQL pagination",
			cli:  "-XPOST graphql --paginate",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "POST",
				RequestMethodPassed: true,
				RequestPath:         "graphql",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            true,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name:     "input pagination",
			cli:      "--input repos/OWNER/REPO/issues --paginate",
			wantsErr: true,
		},
		{
			name: "with request body from file",
			cli:  "user --input myfile",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "user",
				RequestInputFile:    "myfile",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name:     "no arguments",
			cli:      "",
			wantsErr: true,
		},
		{
			name: "with hostname",
			cli:  "graphql --hostname tom.petty",
			wants: ApiOptions{
				Hostname:            "tom.petty",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "graphql",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "with cache",
			cli:  "user --cache 5m",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "user",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            time.Minute * 5,
				Template:            "",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "with template",
			cli:  "user -t 'hello {{.name}}'",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "user",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "hello {{.name}}",
				FilterOutput:        "",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name: "with jq filter",
			cli:  "user -q .name",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "user",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        ".name",
				Verbose:             false,
			},
			wantsErr: false,
		},
		{
			name:     "--silent with --jq",
			cli:      "user --silent -q .foo",
			wantsErr: true,
		},
		{
			name:     "--silent with --template",
			cli:      "user --silent -t '{{.foo}}'",
			wantsErr: true,
		},
		{
			name:     "--jq with --template",
			cli:      "user --jq .foo -t '{{.foo}}'",
			wantsErr: true,
		},
		{
			name:     "--slurp without --paginate",
			cli:      "user --slurp",
			wantsErr: true,
		},
		{
			name:     "slurp with --jq",
			cli:      "user --paginate --slurp --jq .foo",
			wantsErr: true,
		},
		{
			name:     "slurp with --template",
			cli:      "user --paginate --slurp --template '{{.foo}}'",
			wantsErr: true,
		},
		{
			name: "with verbose",
			cli:  "user --verbose",
			wants: ApiOptions{
				Hostname:            "",
				RequestMethod:       "GET",
				RequestMethodPassed: false,
				RequestPath:         "user",
				RequestInputFile:    "",
				RawFields:           []string(nil),
				MagicFields:         []string(nil),
				RequestHeaders:      []string(nil),
				ShowResponseHeaders: false,
				Paginate:            false,
				Silent:              false,
				CacheTTL:            0,
				Template:            "",
				FilterOutput:        "",
				Verbose:             true,
			},
			wantsErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts *ApiOptions
			cmd := NewCmdApi(f, func(o *ApiOptions) error {
				opts = o
				return nil
			})

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Hostname, opts.Hostname)
			assert.Equal(t, tt.wants.RequestMethod, opts.RequestMethod)
			assert.Equal(t, tt.wants.RequestMethodPassed, opts.RequestMethodPassed)
			assert.Equal(t, tt.wants.RequestPath, opts.RequestPath)
			assert.Equal(t, tt.wants.RequestInputFile, opts.RequestInputFile)
			assert.Equal(t, tt.wants.RawFields, opts.RawFields)
			assert.Equal(t, tt.wants.MagicFields, opts.MagicFields)
			assert.Equal(t, tt.wants.RequestHeaders, opts.RequestHeaders)
			assert.Equal(t, tt.wants.ShowResponseHeaders, opts.ShowResponseHeaders)
			assert.Equal(t, tt.wants.Paginate, opts.Paginate)
			assert.Equal(t, tt.wants.Silent, opts.Silent)
			assert.Equal(t, tt.wants.CacheTTL, opts.CacheTTL)
			assert.Equal(t, tt.wants.Template, opts.Template)
			assert.Equal(t, tt.wants.FilterOutput, opts.FilterOutput)
			assert.Equal(t, tt.wants.Verbose, opts.Verbose)
		})
	}
}

func Test_NewCmdApi_WindowsAbsPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.SkipNow()
	}

	cmd := NewCmdApi(&cmdutil.Factory{}, func(opts *ApiOptions) error {
		return nil
	})

	cmd.SetArgs([]string{`C:\users\repos`})
	_, err := cmd.ExecuteC()
	assert.EqualError(t, err, `invalid API endpoint: "C:\users\repos". Your shell might be rewriting URL paths as filesystem paths. To avoid this, omit the leading slash from the endpoint argument`)
}

func Test_apiRun(t *testing.T) {
	tests := []struct {
		name         string
		options      ApiOptions
		httpResponse *http.Response
		err          error
		stdout       string
		stderr       string
		isatty       bool
	}{
		{
			name: "success",
			httpResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`bam!`)),
			},
			err:    nil,
			stdout: `bam!`,
			stderr: ``,
			isatty: false,
		},
		{
			name: "show response headers",
			options: ApiOptions{
				ShowResponseHeaders: true,
			},
			httpResponse: &http.Response{
				Proto:      "HTTP/1.1",
				Status:     "200 Okey-dokey",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`body`)),
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
			},
			err:    nil,
			stdout: "HTTP/1.1 200 Okey-dokey\nContent-Type: text/plain\r\n\r\nbody",
			stderr: ``,
			isatty: false,
		},
		{
			name: "success 204",
			httpResponse: &http.Response{
				StatusCode: 204,
				Body:       nil,
			},
			err:    nil,
			stdout: ``,
			stderr: ``,
			isatty: false,
		},
		{
			name: "REST error",
			httpResponse: &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString(`{"message": "THIS IS FINE"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
			},
			err:    cmdutil.SilentError,
			stdout: `{"message": "THIS IS FINE"}`,
			stderr: "gh: THIS IS FINE (HTTP 400)\n",
			isatty: false,
		},
		{
			name: "REST string errors",
			httpResponse: &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString(`{"errors": ["ALSO", "FINE"]}`)),
				Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
			},
			err:    cmdutil.SilentError,
			stdout: `{"errors": ["ALSO", "FINE"]}`,
			stderr: "gh: ALSO\nFINE\n",
			isatty: false,
		},
		{
			name: "GraphQL error",
			options: ApiOptions{
				RequestPath: "graphql",
			},
			httpResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`{"errors": [{"message":"AGAIN"}, {"message":"FINE"}]}`)),
				Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
			},
			err:    cmdutil.SilentError,
			stdout: `{"errors": [{"message":"AGAIN"}, {"message":"FINE"}]}`,
			stderr: "gh: AGAIN\nFINE\n",
			isatty: false,
		},
		{
			name: "failure",
			httpResponse: &http.Response{
				StatusCode: 502,
				Body:       io.NopCloser(bytes.NewBufferString(`gateway timeout`)),
			},
			err:    cmdutil.SilentError,
			stdout: `gateway timeout`,
			stderr: "gh: HTTP 502\n",
			isatty: false,
		},
		{
			name: "silent",
			options: ApiOptions{
				Silent: true,
			},
			httpResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`body`)),
			},
			err:    nil,
			stdout: ``,
			stderr: ``,
			isatty: false,
		},
		{
			name: "show response headers even when silent",
			options: ApiOptions{
				ShowResponseHeaders: true,
				Silent:              true,
			},
			httpResponse: &http.Response{
				Proto:      "HTTP/1.1",
				Status:     "200 Okey-dokey",
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`body`)),
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
			},
			err:    nil,
			stdout: "HTTP/1.1 200 Okey-dokey\nContent-Type: text/plain\r\n\r\n",
			stderr: ``,
			isatty: false,
		},
		{
			name: "output template",
			options: ApiOptions{
				Template: `{{.status}}`,
			},
			httpResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`{"status":"not a cat"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			},
			err:    nil,
			stdout: "not a cat",
			stderr: ``,
			isatty: false,
		},
		{
			name: "output template with range",
			options: ApiOptions{
				Template: `{{range .}}{{.title}} ({{.labels | pluck "name" | join ", " }}){{"\n"}}{{end}}`,
			},
			httpResponse: &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(`[
					{
						"title": "First title",
						"labels": [{"name":"bug"}, {"name":"help wanted"}]
					},
					{
						"title": "Second but not last"
					},
					{
						"title": "Alas, tis' the end",
						"labels": [{}, {"name":"feature"}]
					}
				]`)),
				Header: http.Header{"Content-Type": []string{"application/json"}},
			},
			stdout: heredoc.Doc(`
			First title (bug, help wanted)
			Second but not last ()
			Alas, tis' the end (, feature)
		`),
		},
		{
			name: "output template when REST error",
			options: ApiOptions{
				Template: `{{.status}}`,
			},
			httpResponse: &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString(`{"message": "THIS IS FINE"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
			},
			err:    cmdutil.SilentError,
			stdout: `{"message": "THIS IS FINE"}`,
			stderr: "gh: THIS IS FINE (HTTP 400)\n",
			isatty: false,
		},
		{
			name: "jq filter",
			options: ApiOptions{
				FilterOutput: `.[].name`,
			},
			httpResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`[{"name":"Mona"},{"name":"Hubot"}]`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			},
			err:    nil,
			stdout: "Mona\nHubot\n",
			stderr: ``,
			isatty: false,
		},
		{
			name: "jq filter when REST error",
			options: ApiOptions{
				FilterOutput: `.[].name`,
			},
			httpResponse: &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewBufferString(`{"message": "THIS IS FINE"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
			},
			err:    cmdutil.SilentError,
			stdout: `{"message": "THIS IS FINE"}`,
			stderr: "gh: THIS IS FINE (HTTP 400)\n",
			isatty: false,
		},
		{
			name: "jq filter outputting JSON to a TTY",
			options: ApiOptions{
				FilterOutput: `.`,
			},
			httpResponse: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(`[{"name":"Mona"},{"name":"Hubot"}]`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			},
			err:    nil,
			stdout: "[\n  {\n    \"name\": \"Mona\"\n  },\n  {\n    \"name\": \"Hubot\"\n  }\n]\n",
			stderr: ``,
			isatty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isatty)

			tt.options.IO = ios
			tt.options.Config = func() (gh.Config, error) { return config.NewBlankConfig(), nil }
			tt.options.HttpClient = func() (*http.Client, error) {
				var tr roundTripper = func(req *http.Request) (*http.Response, error) {
					resp := tt.httpResponse
					resp.Request = req
					return resp, nil
				}
				return &http.Client{Transport: tr}, nil
			}

			err := apiRun(&tt.options)
			if err != tt.err {
				t.Errorf("expected error %v, got %v", tt.err, err)
			}

			if stdout.String() != tt.stdout {
				t.Errorf("expected output %q, got %q", tt.stdout, stdout.String())
			}
			if stderr.String() != tt.stderr {
				t.Errorf("expected error output %q, got %q", tt.stderr, stderr.String())
			}
		})
	}
}

func Test_apiRun_paginationREST(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	requestCount := 0
	responses := []*http.Response{
		{
			Proto:      "HTTP/1.1",
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`{"page":1}`)),
			Header: http.Header{
				"Content-Type":        []string{"application/json"},
				"Link":                []string{`<https://api.github.com/repositories/1227/issues?page=2>; rel="next", <https://api.github.com/repositories/1227/issues?page=3>; rel="last"`},
				"X-Github-Request-Id": []string{"1"},
			},
		},
		{
			Proto:      "HTTP/1.1",
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`{"page":2}`)),
			Header: http.Header{
				"Content-Type":        []string{"application/json"},
				"Link":                []string{`<https://api.github.com/repositories/1227/issues?page=3>; rel="next", <https://api.github.com/repositories/1227/issues?page=3>; rel="last"`},
				"X-Github-Request-Id": []string{"2"},
			},
		},
		{
			Proto:      "HTTP/1.1",
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`{"page":3}`)),
			Header: http.Header{
				"Content-Type":        []string{"application/json"},
				"X-Github-Request-Id": []string{"3"},
			},
		},
	}

	options := ApiOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			var tr roundTripper = func(req *http.Request) (*http.Response, error) {
				resp := responses[requestCount]
				resp.Request = req
				requestCount++
				return resp, nil
			}
			return &http.Client{Transport: tr}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},

		RequestMethod:       "GET",
		RequestMethodPassed: true,
		RequestPath:         "issues",
		Paginate:            true,
		RawFields:           []string{"per_page=50", "page=1"},
	}

	err := apiRun(&options)
	assert.NoError(t, err)

	assert.Equal(t, `{"page":1}{"page":2}{"page":3}`, stdout.String(), "stdout")
	assert.Equal(t, "", stderr.String(), "stderr")

	assert.Equal(t, "https://api.github.com/issues?page=1&per_page=50", responses[0].Request.URL.String())
	assert.Equal(t, "https://api.github.com/repositories/1227/issues?page=2", responses[1].Request.URL.String())
	assert.Equal(t, "https://api.github.com/repositories/1227/issues?page=3", responses[2].Request.URL.String())
}

func Test_apiRun_arrayPaginationREST(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(false)

	requestCount := 0
	responses := []*http.Response{
		{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`[{"item":1},{"item":2}]`)),
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"Link":         []string{`<https://api.github.com/repositories/1227/issues?page=2>; rel="next", <https://api.github.com/repositories/1227/issues?page=4>; rel="last"`},
			},
		},
		{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`[{"item":3},{"item":4}]`)),
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"Link":         []string{`<https://api.github.com/repositories/1227/issues?page=3>; rel="next", <https://api.github.com/repositories/1227/issues?page=4>; rel="last"`},
			},
		},
		{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`[{"item":5}]`)),
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"Link":         []string{`<https://api.github.com/repositories/1227/issues?page=4>; rel="next", <https://api.github.com/repositories/1227/issues?page=4>; rel="last"`},
			},
		},
		{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`[]`)),
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
	}

	options := ApiOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			var tr roundTripper = func(req *http.Request) (*http.Response, error) {
				resp := responses[requestCount]
				resp.Request = req
				requestCount++
				return resp, nil
			}
			return &http.Client{Transport: tr}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},

		RequestMethod:       "GET",
		RequestMethodPassed: true,
		RequestPath:         "issues",
		Paginate:            true,
		RawFields:           []string{"per_page=50", "page=1"},
	}

	err := apiRun(&options)
	assert.NoError(t, err)

	assert.Equal(t, `[{"item":1},{"item":2},{"item":3},{"item":4},{"item":5} ]`, stdout.String(), "stdout")
	assert.Equal(t, "", stderr.String(), "stderr")

	assert.Equal(t, "https://api.github.com/issues?page=1&per_page=50", responses[0].Request.URL.String())
	assert.Equal(t, "https://api.github.com/repositories/1227/issues?page=2", responses[1].Request.URL.String())
	assert.Equal(t, "https://api.github.com/repositories/1227/issues?page=3", responses[2].Request.URL.String())
}

func Test_apiRun_arrayPaginationREST_with_headers(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	requestCount := 0
	responses := []*http.Response{
		{
			Proto:      "HTTP/1.1",
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`[{"page":1}]`)),
			Header: http.Header{
				"Content-Type":        []string{"application/json"},
				"Link":                []string{`<https://api.github.com/repositories/1227/issues?page=2>; rel="next", <https://api.github.com/repositories/1227/issues?page=3>; rel="last"`},
				"X-Github-Request-Id": []string{"1"},
			},
		},
		{
			Proto:      "HTTP/1.1",
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`[{"page":2}]`)),
			Header: http.Header{
				"Content-Type":        []string{"application/json"},
				"Link":                []string{`<https://api.github.com/repositories/1227/issues?page=3>; rel="next", <https://api.github.com/repositories/1227/issues?page=3>; rel="last"`},
				"X-Github-Request-Id": []string{"2"},
			},
		},
		{
			Proto:      "HTTP/1.1",
			Status:     "200 OK",
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(`[{"page":3}]`)),
			Header: http.Header{
				"Content-Type":        []string{"application/json"},
				"X-Github-Request-Id": []string{"3"},
			},
		},
	}

	options := ApiOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			var tr roundTripper = func(req *http.Request) (*http.Response, error) {
				resp := responses[requestCount]
				resp.Request = req
				requestCount++
				return resp, nil
			}
			return &http.Client{Transport: tr}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},

		RequestMethod:       "GET",
		RequestMethodPassed: true,
		RequestPath:         "issues",
		Paginate:            true,
		RawFields:           []string{"per_page=50", "page=1"},
		ShowResponseHeaders: true,
	}

	err := apiRun(&options)
	assert.NoError(t, err)

	assert.Equal(t, "HTTP/1.1 200 OK\nContent-Type: application/json\r\nLink: <https://api.github.com/repositories/1227/issues?page=2>; rel=\"next\", <https://api.github.com/repositories/1227/issues?page=3>; rel=\"last\"\r\nX-Github-Request-Id: 1\r\n\r\n[{\"page\":1}]\nHTTP/1.1 200 OK\nContent-Type: application/json\r\nLink: <https://api.github.com/repositories/1227/issues?page=3>; rel=\"next\", <https://api.github.com/repositories/1227/issues?page=3>; rel=\"last\"\r\nX-Github-Request-Id: 2\r\n\r\n[{\"page\":2}]\nHTTP/1.1 200 OK\nContent-Type: application/json\r\nX-Github-Request-Id: 3\r\n\r\n[{\"page\":3}]", stdout.String(), "stdout")
	assert.Equal(t, "", stderr.String(), "stderr")

	assert.Equal(t, "https://api.github.com/issues?page=1&per_page=50", responses[0].Request.URL.String())
	assert.Equal(t, "https://api.github.com/repositories/1227/issues?page=2", responses[1].Request.URL.String())
	assert.Equal(t, "https://api.github.com/repositories/1227/issues?page=3", responses[2].Request.URL.String())
}

func Test_apiRun_paginationGraphQL(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	requestCount := 0
	responses := []*http.Response{
		{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{`application/json`}},
			Body: io.NopCloser(bytes.NewBufferString(heredoc.Doc(`
			{
				"data": {
					"nodes": ["page one"],
					"pageInfo": {
						"endCursor": "PAGE1_END",
						"hasNextPage": true
					}
				}
			}`))),
		},
		{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{`application/json`}},
			Body: io.NopCloser(bytes.NewBufferString(heredoc.Doc(`
			{
				"data": {
					"nodes": ["page two"],
					"pageInfo": {
						"endCursor": "PAGE2_END",
						"hasNextPage": false
					}
				}
			}`))),
		},
	}

	options := ApiOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			var tr roundTripper = func(req *http.Request) (*http.Response, error) {
				resp := responses[requestCount]
				resp.Request = req
				requestCount++
				return resp, nil
			}
			return &http.Client{Transport: tr}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},

		RawFields:     []string{"foo=bar"},
		RequestMethod: "POST",
		RequestPath:   "graphql",
		Paginate:      true,
	}

	err := apiRun(&options)
	require.NoError(t, err)

	assert.Equal(t, heredoc.Doc(`
	{
		"data": {
			"nodes": ["page one"],
			"pageInfo": {
				"endCursor": "PAGE1_END",
				"hasNextPage": true
			}
		}
	}{
		"data": {
			"nodes": ["page two"],
			"pageInfo": {
				"endCursor": "PAGE2_END",
				"hasNextPage": false
			}
		}
	}`), stdout.String())
	assert.Equal(t, "", stderr.String(), "stderr")

	var requestData struct {
		Variables map[string]interface{}
	}

	bb, err := io.ReadAll(responses[0].Request.Body)
	require.NoError(t, err)
	err = json.Unmarshal(bb, &requestData)
	require.NoError(t, err)
	_, hasCursor := requestData.Variables["endCursor"].(string)
	assert.Equal(t, false, hasCursor)

	bb, err = io.ReadAll(responses[1].Request.Body)
	require.NoError(t, err)
	err = json.Unmarshal(bb, &requestData)
	require.NoError(t, err)
	endCursor, hasCursor := requestData.Variables["endCursor"].(string)
	assert.Equal(t, true, hasCursor)
	assert.Equal(t, "PAGE1_END", endCursor)
}

func Test_apiRun_paginationGraphQL_slurp(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	requestCount := 0
	responses := []*http.Response{
		{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{`application/json`}},
			Body: io.NopCloser(bytes.NewBufferString(heredoc.Doc(`
			{
				"data": {
					"nodes": ["page one"],
					"pageInfo": {
						"endCursor": "PAGE1_END",
						"hasNextPage": true
					}
				}
			}`))),
		},
		{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{`application/json`}},
			Body: io.NopCloser(bytes.NewBufferString(heredoc.Doc(`
			{
				"data": {
					"nodes": ["page two"],
					"pageInfo": {
						"endCursor": "PAGE2_END",
						"hasNextPage": false
					}
				}
			}`))),
		},
	}

	options := ApiOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			var tr roundTripper = func(req *http.Request) (*http.Response, error) {
				resp := responses[requestCount]
				resp.Request = req
				requestCount++
				return resp, nil
			}
			return &http.Client{Transport: tr}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},

		RawFields:     []string{"foo=bar"},
		RequestMethod: "POST",
		RequestPath:   "graphql",
		Paginate:      true,
		Slurp:         true,
	}

	err := apiRun(&options)
	require.NoError(t, err)

	assert.JSONEq(t, stdout.String(), `[
		{
			"data": {
				"nodes": ["page one"],
				"pageInfo": {
					"endCursor": "PAGE1_END",
					"hasNextPage": true
				}
			}
		},
		{

			"data": {
				"nodes": ["page two"],
				"pageInfo": {
					"endCursor": "PAGE2_END",
					"hasNextPage": false
				}
			}
		}
	]`)
	assert.Equal(t, "", stderr.String(), "stderr")

	var requestData struct {
		Variables map[string]interface{}
	}

	bb, err := io.ReadAll(responses[0].Request.Body)
	require.NoError(t, err)
	err = json.Unmarshal(bb, &requestData)
	require.NoError(t, err)
	_, hasCursor := requestData.Variables["endCursor"].(string)
	assert.Equal(t, false, hasCursor)

	bb, err = io.ReadAll(responses[1].Request.Body)
	require.NoError(t, err)
	err = json.Unmarshal(bb, &requestData)
	require.NoError(t, err)
	endCursor, hasCursor := requestData.Variables["endCursor"].(string)
	assert.Equal(t, true, hasCursor)
	assert.Equal(t, "PAGE1_END", endCursor)
}

func Test_apiRun_paginated_template(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)

	requestCount := 0
	responses := []*http.Response{
		{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{`application/json`}},
			Body: io.NopCloser(bytes.NewBufferString(`{
				"data": {
					"nodes": [
						{
							"page": 1,
							"caption": "page one"
						}
					],
					"pageInfo": {
						"endCursor": "PAGE1_END",
						"hasNextPage": true
					}
				}
			}`)),
		},
		{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{`application/json`}},
			Body: io.NopCloser(bytes.NewBufferString(`{
				"data": {
					"nodes": [
						{
							"page": 20,
							"caption": "page twenty"
						}
					],
					"pageInfo": {
						"endCursor": "PAGE20_END",
						"hasNextPage": false
					}
				}
			}`)),
		},
	}

	options := ApiOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			var tr roundTripper = func(req *http.Request) (*http.Response, error) {
				resp := responses[requestCount]
				resp.Request = req
				requestCount++
				return resp, nil
			}
			return &http.Client{Transport: tr}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},

		RequestMethod: "POST",
		RequestPath:   "graphql",
		RawFields:     []string{"foo=bar"},
		Paginate:      true,
		// test that templates executed per page properly render a table.
		Template: `{{range .data.nodes}}{{tablerow .page .caption}}{{end}}`,
	}

	err := apiRun(&options)
	require.NoError(t, err)

	assert.Equal(t, heredoc.Doc(`
	1   page one
	20  page twenty
	`), stdout.String(), "stdout")
	assert.Equal(t, "", stderr.String(), "stderr")

	var requestData struct {
		Variables map[string]interface{}
	}

	bb, err := io.ReadAll(responses[0].Request.Body)
	require.NoError(t, err)
	err = json.Unmarshal(bb, &requestData)
	require.NoError(t, err)
	_, hasCursor := requestData.Variables["endCursor"].(string)
	assert.Equal(t, false, hasCursor)

	bb, err = io.ReadAll(responses[1].Request.Body)
	require.NoError(t, err)
	err = json.Unmarshal(bb, &requestData)
	require.NoError(t, err)
	endCursor, hasCursor := requestData.Variables["endCursor"].(string)
	assert.Equal(t, true, hasCursor)
	assert.Equal(t, "PAGE1_END", endCursor)
}

func Test_apiRun_DELETE(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	var gotRequest *http.Request
	err := apiRun(&ApiOptions{
		IO: ios,
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		HttpClient: func() (*http.Client, error) {
			var tr roundTripper = func(req *http.Request) (*http.Response, error) {
				gotRequest = req
				return &http.Response{StatusCode: 204, Request: req}, nil
			}
			return &http.Client{Transport: tr}, nil
		},
		MagicFields:         []string(nil),
		RawFields:           []string(nil),
		RequestMethod:       "DELETE",
		RequestMethodPassed: true,
	})
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	if gotRequest.Body != nil {
		t.Errorf("expected nil request body, got %T", gotRequest.Body)
	}
}

func Test_apiRun_inputFile(t *testing.T) {
	tests := []struct {
		name          string
		inputFile     string
		inputContents []byte

		contentLength    int64
		expectedContents []byte
	}{
		{
			name:          "stdin",
			inputFile:     "-",
			inputContents: []byte("I WORK OUT"),
			contentLength: 0,
		},
		{
			name:          "from file",
			inputFile:     "gh-test-file",
			inputContents: []byte("I WORK OUT"),
			contentLength: 10,
		},
	}

	tempDir := t.TempDir()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			resp := &http.Response{StatusCode: 204}

			inputFile := tt.inputFile
			if tt.inputFile == "-" {
				_, _ = stdin.Write(tt.inputContents)
			} else {
				f, err := os.CreateTemp(tempDir, tt.inputFile)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = f.Write(tt.inputContents)
				defer f.Close()
				inputFile = f.Name()
			}

			var bodyBytes []byte
			options := ApiOptions{
				RequestPath:      "hello",
				RequestInputFile: inputFile,
				RawFields:        []string{"a=b", "c=d"},

				IO: ios,
				HttpClient: func() (*http.Client, error) {
					var tr roundTripper = func(req *http.Request) (*http.Response, error) {
						var err error
						if bodyBytes, err = io.ReadAll(req.Body); err != nil {
							return nil, err
						}
						resp.Request = req
						return resp, nil
					}
					return &http.Client{Transport: tr}, nil
				},
				Config: func() (gh.Config, error) {
					return config.NewBlankConfig(), nil
				},
			}

			err := apiRun(&options)
			if err != nil {
				t.Errorf("got error %v", err)
			}

			assert.Equal(t, "POST", resp.Request.Method)
			assert.Equal(t, "/hello?a=b&c=d", resp.Request.URL.RequestURI())
			assert.Equal(t, tt.contentLength, resp.Request.ContentLength)
			assert.Equal(t, "", resp.Request.Header.Get("Content-Type"))
			assert.Equal(t, tt.inputContents, bodyBytes)
		})
	}
}

func Test_apiRun_cache(t *testing.T) {
	// Given we have a test server that spies on the number of requests it receives
	requestCount := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(s.Close)

	ios, _, stdout, stderr := iostreams.Test()
	options := ApiOptions{
		IO: ios,
		Config: func() (gh.Config, error) {
			return &ghmock.ConfigMock{
				AuthenticationFunc: func() gh.AuthConfig {
					return &config.AuthConfig{}
				},
				// Cached responses are stored in a tempdir that gets automatically cleaned up
				CacheDirFunc: func() string {
					return t.TempDir()
				},
			}, nil
		},
		// You might think that we want to set Host: s.URL here, but you'd be wrong.
		// The host field is later used to evaluate an API URL e.g. https://api.host.com/graphql
		// The RequestPath field is used exactly as is, for the request if it includes a host.
		RequestPath: s.URL,
		CacheTTL:    time.Minute,
	}

	// When we run the API behaviour twice
	require.NoError(t, apiRun(&options))
	require.NoError(t, apiRun(&options))

	// We only get one request to the http server because it uses the cached response
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, "", stdout.String(), "stdout")
	assert.Equal(t, "", stderr.String(), "stderr")
}

func Test_openUserFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "gh-test")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	fmt.Fprint(f, "file contents")

	file, length, err := openUserFile(f.Name(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	fb, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, int64(13), length)
	assert.Equal(t, "file contents", string(fb))
}

func Test_fillPlaceholders(t *testing.T) {
	type args struct {
		value string
		opts  *ApiOptions
	}
	tests := []struct {
		name         string
		args         args
		repoOverride bool
		want         string
		wantErr      bool
	}{
		{
			name: "no changes",
			args: args{
				value: "repos/owner/repo/releases",
				opts: &ApiOptions{
					BaseRepo: nil,
				},
			},
			want:    "repos/owner/repo/releases",
			wantErr: false,
		},
		{
			name: "has substitutes (colon)",
			args: args{
				value: "repos/:owner/:repo/releases",
				opts: &ApiOptions{
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
				},
			},
			want:    "repos/hubot/robot-uprising/releases",
			wantErr: false,
		},
		{
			name: "has branch placeholder (colon)",
			args: args{
				value: "repos/owner/repo/branches/:branch/protection/required_status_checks",
				opts: &ApiOptions{
					BaseRepo: nil,
					Branch: func() (string, error) {
						return "trunk", nil
					},
				},
			},
			want:    "repos/owner/repo/branches/trunk/protection/required_status_checks",
			wantErr: false,
		},
		{
			name: "has branch placeholder and git is in detached head (colon)",
			args: args{
				value: "repos/:owner/:repo/branches/:branch",
				opts: &ApiOptions{
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
					Branch: func() (string, error) {
						return "", git.ErrNotOnAnyBranch
					},
				},
			},
			want:    "repos/hubot/robot-uprising/branches/:branch",
			wantErr: true,
		},
		{
			name: "has substitutes",
			args: args{
				value: "repos/{owner}/{repo}/releases",
				opts: &ApiOptions{
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
				},
			},
			want:    "repos/hubot/robot-uprising/releases",
			wantErr: false,
		},
		{
			name: "has branch placeholder",
			args: args{
				value: "repos/owner/repo/branches/{branch}/protection/required_status_checks",
				opts: &ApiOptions{
					BaseRepo: nil,
					Branch: func() (string, error) {
						return "trunk", nil
					},
				},
			},
			want:    "repos/owner/repo/branches/trunk/protection/required_status_checks",
			wantErr: false,
		},
		{
			name: "has branch placeholder and git is in detached head",
			args: args{
				value: "repos/{owner}/{repo}/branches/{branch}",
				opts: &ApiOptions{
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
					Branch: func() (string, error) {
						return "", git.ErrNotOnAnyBranch
					},
				},
			},
			want:    "repos/hubot/robot-uprising/branches/{branch}",
			wantErr: true,
		},
		{
			name: "surfaces errors in earlier placeholders",
			args: args{
				value: "{branch}-{owner}",
				opts: &ApiOptions{
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
					Branch: func() (string, error) {
						return "", git.ErrNotOnAnyBranch
					},
				},
			},
			want:    "{branch}-hubot",
			wantErr: true,
		},
		{
			name: "no greedy substitutes (colon)",
			args: args{
				value: ":ownership/:repository",
				opts: &ApiOptions{
					BaseRepo: nil,
				},
			},
			want:    ":ownership/:repository",
			wantErr: false,
		},
		{
			name: "non-placeholders are left intact",
			args: args{
				value: "{}{ownership}/{repository}",
				opts: &ApiOptions{
					BaseRepo: nil,
				},
			},
			want:    "{}{ownership}/{repository}",
			wantErr: false,
		},
		{
			name:         "branch can't be filled when GH_REPO is set",
			repoOverride: true,
			args: args{
				value: "repos/:owner/:repo/branches/:branch",
				opts: &ApiOptions{
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
				},
			},
			want:    "repos/hubot/robot-uprising/branches/:branch",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.repoOverride {
				t.Setenv("GH_REPO", "hubot/robot-uprising")
			}
			got, err := fillPlaceholders(tt.args.value, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("fillPlaceholders() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("fillPlaceholders() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_previewNamesToMIMETypes(t *testing.T) {
	tests := []struct {
		name     string
		previews []string
		want     string
	}{
		{
			name:     "single",
			previews: []string{"nebula"},
			want:     "application/vnd.github.nebula-preview+json",
		},
		{
			name:     "multiple",
			previews: []string{"nebula", "baptiste", "squirrel-girl"},
			want:     "application/vnd.github.nebula-preview+json, application/vnd.github.baptiste-preview, application/vnd.github.squirrel-girl-preview",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := previewNamesToMIMETypes(tt.previews); got != tt.want {
				t.Errorf("previewNamesToMIMETypes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_processResponse_template(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	resp := http.Response{
		StatusCode: 200,
		Header: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(`[
			{
				"title": "First title",
				"labels": [{"name":"bug"}, {"name":"help wanted"}]
			},
			{
				"title": "Second but not last"
			},
			{
				"title": "Alas, tis' the end",
				"labels": [{}, {"name":"feature"}]
			}
		]`)),
	}

	opts := ApiOptions{
		IO:       ios,
		Template: `{{range .}}{{.title}} ({{.labels | pluck "name" | join ", " }}){{"\n"}}{{end}}`,
	}

	tmpl := template.New(ios.Out, ios.TerminalWidth(), ios.ColorEnabled())
	err := tmpl.Parse(opts.Template)
	require.NoError(t, err)
	_, err = processResponse(&resp, &opts, ios.Out, io.Discard, tmpl, true, true)
	require.NoError(t, err)
	err = tmpl.Flush()
	require.NoError(t, err)

	assert.Equal(t, heredoc.Doc(`
		First title (bug, help wanted)
		Second but not last ()
		Alas, tis' the end (, feature)
	`), stdout.String())
	assert.Equal(t, "", stderr.String())
}

func Test_parseErrorResponse(t *testing.T) {
	type args struct {
		input      string
		statusCode int
	}
	tests := []struct {
		name       string
		args       args
		wantErrMsg string
		wantErr    bool
	}{
		{
			name: "no error",
			args: args{
				input:      `{}`,
				statusCode: 500,
			},
			wantErrMsg: "",
			wantErr:    false,
		},
		{
			name: "nil errors",
			args: args{
				input:      `{"errors":null}`,
				statusCode: 500,
			},
			wantErrMsg: "",
			wantErr:    false,
		},
		{
			name: "simple error",
			args: args{
				input:      `{"message": "OH NOES"}`,
				statusCode: 500,
			},
			wantErrMsg: "OH NOES (HTTP 500)",
			wantErr:    false,
		},
		{
			name: "errors string",
			args: args{
				input:      `{"message": "Conflict", "errors": "Some description"}`,
				statusCode: 409,
			},
			wantErrMsg: "Some description (Conflict)",
			wantErr:    false,
		},
		{
			name: "errors array of strings",
			args: args{
				input:      `{"errors": ["fail1", "asplode2"]}`,
				statusCode: 500,
			},
			wantErrMsg: "fail1\nasplode2",
			wantErr:    false,
		},
		{
			name: "errors array of objects",
			args: args{
				input:      `{"errors": [{"message":"fail1"}, {"message":"asplode2"}]}`,
				statusCode: 500,
			},
			wantErrMsg: "fail1\nasplode2",
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := parseErrorResponse(strings.NewReader(tt.args.input), tt.args.statusCode)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseErrorResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotString, _ := io.ReadAll(got); tt.args.input != string(gotString) {
				t.Errorf("parseErrorResponse() got = %q, want %q", string(gotString), tt.args.input)
			}
			if got1 != tt.wantErrMsg {
				t.Errorf("parseErrorResponse() got1 = %q, want %q", got1, tt.wantErrMsg)
			}
		})
	}
}

func Test_apiRun_acceptHeader(t *testing.T) {
	tests := []struct {
		name             string
		options          ApiOptions
		wantAcceptHeader string
	}{
		{
			name:             "sets default accept header",
			options:          ApiOptions{},
			wantAcceptHeader: "*/*",
		},
		{
			name: "does not override user accept header",
			options: ApiOptions{
				RequestHeaders: []string{"Accept: testing"},
			},
			wantAcceptHeader: "testing",
		},
		{
			name: "does not override preview names",
			options: ApiOptions{
				Previews: []string{"nebula"},
			},
			wantAcceptHeader: "application/vnd.github.nebula-preview+json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			tt.options.IO = ios

			tt.options.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}

			var gotReq *http.Request
			tt.options.HttpClient = func() (*http.Client, error) {
				var tr roundTripper = func(req *http.Request) (*http.Response, error) {
					gotReq = req
					resp := &http.Response{
						StatusCode: 200,
						Request:    req,
						Body:       io.NopCloser(bytes.NewBufferString("")),
					}
					return resp, nil
				}
				return &http.Client{Transport: tr}, nil
			}

			assert.NoError(t, apiRun(&tt.options))
			assert.Equal(t, tt.wantAcceptHeader, gotReq.Header.Get("Accept"))
		})
	}
}
