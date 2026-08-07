package api

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_groupGraphQLVariables(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want map[string]any
	}{
		{
			name: "empty",
			args: map[string]any{},
			want: map[string]any{},
		},
		{
			name: "query only",
			args: map[string]any{
				"query": "QUERY",
			},
			want: map[string]any{
				"query": "QUERY",
			},
		},
		{
			name: "variables only",
			args: map[string]any{
				"name": "hubot",
			},
			want: map[string]any{
				"variables": map[string]any{
					"name": "hubot",
				},
			},
		},
		{
			name: "query + variables",
			args: map[string]any{
				"query": "QUERY",
				"name":  "hubot",
				"power": 9001,
			},
			want: map[string]any{
				"query": "QUERY",
				"variables": map[string]any{
					"name":  "hubot",
					"power": 9001,
				},
			},
		},
		{
			name: "query + operationName + variables",
			args: map[string]any{
				"query":         "query Q1{} query Q2{}",
				"operationName": "Q1",
				"power":         9001,
			},
			want: map[string]any{
				"query":         "query Q1{} query Q2{}",
				"operationName": "Q1",
				"variables": map[string]any{
					"power": 9001,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupGraphQLVariables(tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func Test_httpRequest(t *testing.T) {
	var tr roundTripper = func(req *http.Request) (*http.Response, error) {
		return &http.Response{Request: req}, nil
	}
	httpClient := http.Client{Transport: tr}

	type args struct {
		client  *http.Client
		host    string
		apiHost string
		method  string
		p       string
		params  any
		headers []string
	}
	type expects struct {
		method        string
		u             string
		path          string // decoded path, checked when non-empty
		rawURLString  string // encoded URL from req.URL.String(), checked when non-empty
		body          string
		headers       string
		contentLength int64
	}
	tests := []struct {
		name    string
		args    args
		want    expects
		wantErr bool
	}{
		{
			name: "simple GET",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				method:  "GET",
				p:       "repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "GET",
				u:       "https://api.github.com/repos/octocat/spoon-knife",
				body:    "",
				headers: "Accept: */*\r\n",
			},
		},
		{
			name: "GET with accept header",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				method:  "GET",
				p:       "repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{"Accept: testing"},
			},
			wantErr: false,
			want: expects{
				method:  "GET",
				u:       "https://api.github.com/repos/octocat/spoon-knife",
				body:    "",
				headers: "Accept: testing\r\n",
			},
		},
		{
			name: "lowercase HTTP method",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				method:  "get",
				p:       "repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "GET",
				u:       "https://api.github.com/repos/octocat/spoon-knife",
				body:    "",
				headers: "Accept: */*\r\n",
			},
		},
		{
			name: "GET with leading slash",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				method:  "GET",
				p:       "/repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "GET",
				u:       "https://api.github.com/repos/octocat/spoon-knife",
				body:    "",
				headers: "Accept: */*\r\n",
			},
		},
		{
			name: "Enterprise REST",
			args: args{
				client:  &httpClient,
				host:    "example.org",
				method:  "GET",
				p:       "repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "GET",
				u:       "https://example.org/api/v3/repos/octocat/spoon-knife",
				body:    "",
				headers: "Accept: */*\r\n",
			},
		},
		{
			name: "GET with params",
			args: args{
				client: &httpClient,
				host:   "github.com",
				method: "GET",
				p:      "repos/octocat/spoon-knife",
				params: map[string]any{
					"a": "b",
				},
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "GET",
				u:       "https://api.github.com/repos/octocat/spoon-knife?a=b",
				body:    "",
				headers: "Accept: */*\r\n",
			},
		},
		{
			name: "POST with params",
			args: args{
				client: &httpClient,
				host:   "github.com",
				method: "POST",
				p:      "repos",
				params: map[string]any{
					"a": "b",
				},
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "POST",
				u:       "https://api.github.com/repos",
				body:    `{"a":"b"}`,
				headers: "Accept: */*\r\nContent-Type: application/json; charset=utf-8\r\n",
			},
		},
		{
			name: "POST GraphQL",
			args: args{
				client: &httpClient,
				host:   "github.com",
				method: "POST",
				p:      "graphql",
				params: map[string]any{
					"a": "b",
				},
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "POST",
				u:       "https://api.github.com/graphql",
				body:    `{"variables":{"a":"b"}}`,
				headers: "Accept: */*\r\nContent-Type: application/json; charset=utf-8\r\n",
			},
		},
		{
			name: "Enterprise GraphQL",
			args: args{
				client:  &httpClient,
				host:    "example.org",
				method:  "POST",
				p:       "graphql",
				params:  map[string]any{},
				headers: []string{},
			},
			wantErr: false,
			want: expects{
				method:  "POST",
				u:       "https://example.org/api/graphql",
				body:    `{}`,
				headers: "Accept: */*\r\nContent-Type: application/json; charset=utf-8\r\n",
			},
		},
		{
			name: "POST with body and type",
			args: args{
				client: &httpClient,
				host:   "github.com",
				method: "POST",
				p:      "repos",
				params: bytes.NewBufferString("CUSTOM"),
				headers: []string{
					"content-type: text/plain",
					"accept: application/json",
				},
			},
			wantErr: false,
			want: expects{
				method:  "POST",
				u:       "https://api.github.com/repos",
				body:    `CUSTOM`,
				headers: "Accept: application/json\r\nContent-Type: text/plain\r\n",
			},
		},
		{
			name: "relative REST path with api_host: host is swapped",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				apiHost: "api.mygateway.example",
				method:  "GET",
				p:       "repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{},
			},
			want: expects{
				method:  "GET",
				u:       "https://api.mygateway.example/repos/octocat/spoon-knife",
				headers: "Accept: */*\r\n",
			},
		},
		{
			name: "graphql with api_host: host is swapped",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				apiHost: "api.mygateway.example",
				method:  "POST",
				p:       "graphql",
				params:  map[string]interface{}{},
				headers: []string{},
			},
			want: expects{
				method:  "POST",
				u:       "https://api.mygateway.example/graphql",
				body:    "{}",
				headers: "Accept: */*\r\nContent-Type: application/json; charset=utf-8\r\n",
			},
		},
		{
			name: "absolute URL with api_host: URL is unchanged",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				apiHost: "api.mygateway.example",
				method:  "GET",
				p:       "https://api.github.com/repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{},
			},
			want: expects{
				method:  "GET",
				u:       "https://api.github.com/repos/octocat/spoon-knife",
				headers: "Accept: */*\r\n",
			},
		},
		{
			name: "relative path without api_host: unchanged",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				method:  "GET",
				p:       "repos/octocat/spoon-knife",
				params:  nil,
				headers: []string{},
			},
			want: expects{
				method:  "GET",
				u:       "https://api.github.com/repos/octocat/spoon-knife",
				headers: "Accept: */*\r\n",
			},
		},
		{
			// gh api takes the path verbatim - characters that safeurl would encode
			// must pass through unchanged. This test uses a space and brackets,
			// which safeurl would percent-encode.
			name: "path with characters safeurl would escape: passed verbatim",
			args: args{
				client:  &httpClient,
				host:    "github.com",
				method:  "GET",
				p:       "repos/octocat/hello world[0]",
				params:  nil,
				headers: []string{},
			},
			want: expects{
				method:       "GET",
				path:         "/repos/octocat/hello world[0]",
				rawURLString: "https://api.github.com/repos/octocat/hello%20world%5B0%5D",
				headers:      "Accept: */*\r\n",
			},
		},
		{
			name: "Content-Length header sets req.ContentLength",
			args: args{
				client: &httpClient,
				host:   "github.com",
				method: "POST",
				p:      "repos",
				params: bytes.NewBufferString("BODY"),
				headers: []string{
					"Content-Length: 4",
				},
			},
			want: expects{
				method:        "POST",
				u:             "https://api.github.com/repos",
				body:          "BODY",
				headers:       "Accept: */*\r\n",
				contentLength: 4,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := httpRequest(tt.args.client, tt.args.host, tt.args.apiHost, tt.args.method, tt.args.p, tt.args.params, tt.args.headers)
			if (err != nil) != tt.wantErr {
				t.Errorf("httpRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			req := got.Request
			if req.Method != tt.want.method {
				t.Errorf("Request.Method = %q, want %q", req.Method, tt.want.method)
			}
			if tt.want.u != "" && req.URL.String() != tt.want.u {
				t.Errorf("Request.URL = %q, want %q", req.URL.String(), tt.want.u)
			}
			if tt.want.path != "" && req.URL.Path != tt.want.path {
				t.Errorf("Request.URL.Path = %q, want %q", req.URL.Path, tt.want.path)
			}
			if tt.want.rawURLString != "" && req.URL.String() != tt.want.rawURLString {
				t.Errorf("Request.URL.String() = %q, want %q", req.URL.String(), tt.want.rawURLString)
			}

			if tt.want.body != "" {
				bb, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("Request.Body ReadAll error = %v", err)
					return
				}
				if string(bb) != tt.want.body {
					t.Errorf("Request.Body = %q, want %q", string(bb), tt.want.body)
				}
			}

			h := bytes.Buffer{}
			err = req.Header.WriteSubset(&h, map[string]bool{})
			if err != nil {
				t.Errorf("Request.Header WriteSubset error = %v", err)
				return
			}
			if h.String() != tt.want.headers {
				t.Errorf("Request.Header = %q, want %q", h.String(), tt.want.headers)
			}
			if tt.want.contentLength != 0 && req.ContentLength != tt.want.contentLength {
				t.Errorf("Request.ContentLength = %d, want %d", req.ContentLength, tt.want.contentLength)
			}
		})
	}
}

func Test_addQuery(t *testing.T) {
	type args struct {
		path   string
		params map[string]any
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "string",
			args: args{
				path:   "",
				params: map[string]any{"a": "hello"},
			},
			want: "?a=hello",
		},
		{
			name: "array",
			args: args{
				path:   "",
				params: map[string]any{"a": []any{"hello", "world"}},
			},
			want: "?a%5B%5D=hello&a%5B%5D=world",
		},
		{
			name: "append",
			args: args{
				path:   "path",
				params: map[string]any{"a": "b"},
			},
			want: "path?a=b",
		},
		{
			name: "append query",
			args: args{
				path:   "path?foo=bar",
				params: map[string]any{"a": "b"},
			},
			want: "path?foo=bar&a=b",
		},
		{
			name: "[]byte",
			args: args{
				path:   "",
				params: map[string]any{"a": []byte("hello")},
			},
			want: "?a=hello",
		},
		{
			name: "int",
			args: args{
				path:   "",
				params: map[string]any{"a": 123},
			},
			want: "?a=123",
		},
		{
			name: "nil",
			args: args{
				path:   "",
				params: map[string]any{"a": nil},
			},
			want: "?a=",
		},
		{
			name: "bool",
			args: args{
				path:   "",
				params: map[string]any{"a": true, "b": false},
			},
			want: "?a=true&b=false",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := addQuery(tt.args.path, tt.args.params); got != tt.want {
				t.Errorf("addQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
