package list

import (
	"bytes"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
)

func TestCmdList(t *testing.T) {
	io, _, _, _ := iostreams.Test()
	io.SetStdoutTTY(true)
	io.SetStdinTTY(true)
	io.SetStderrTTY(true)

	httpFunc := func() (*http.Client, error) { return nil, nil }
	configFunc := func() (config.Config, error) { return nil, nil }

	type input struct {
		cli        string
		httpClient func() (*http.Client, error)
		io         *iostreams.IOStreams
		config     func() (config.Config, error)
	}

	tests := []struct {
		name  string
		input input
		wants ListOptions
	}{
		{
			name: "no arguments",
			input: input{
				cli:        "",
				httpClient: httpFunc,
				io:         io,
				config:     configFunc,
			},
			wants: ListOptions{
				HTTPClient: httpFunc,
				Config:     configFunc,
				IO:         io,
				ListMsg:    []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{
				HttpClient: tt.input.httpClient,
				Config:     tt.input.config,
				IOStreams:  tt.input.io,
			}

			argv, err := shlex.Split(tt.input.cli)
			if err != nil {
				t.Errorf(`Split() = got %v`, err)
			}

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if err != nil {
				t.Errorf(`ExecuteC() = got %v`, err)
			}

			if reflect.ValueOf(tt.wants.HTTPClient).Pointer() != reflect.ValueOf(gotOpts.HTTPClient).Pointer() {
				t.Errorf(`HTTPClient has wrong values`)
			}

			if reflect.ValueOf(tt.wants.Config).Pointer() != reflect.ValueOf(gotOpts.Config).Pointer() {
				t.Errorf(`Config has wrong values`)
			}

			if reflect.ValueOf(tt.wants.IO).Pointer() != reflect.ValueOf(gotOpts.IO).Pointer() {
				t.Errorf(`IO has wrong values`)
			}

			if !reflect.DeepEqual(tt.wants.ListMsg, gotOpts.ListMsg) {
				t.Errorf(`ListMsg has wrong values: want %v, got %v`, tt.wants.ListMsg, gotOpts.ListMsg)
			}
		})
	}
}

func TestListRun(t *testing.T) {
	type input struct {
		httpStubs       func(*httpmock.Registry)
		configError     bool
		httpClientError bool
		hasOauthToken   bool
		wantErr         bool
	}

	tests := []struct {
		name  string
		input input
		want  []string
	}{
		{
			name: "name and corresponding ssh key",
			input: input{
				func(reg *httpmock.Registry) {
					reg.Register(
						httpmock.REST("GET", "user/keys"),
						httpmock.StringResponse(`[{"id":1234,"key":"ssh-rsa AAAABbBB123","title":"Mac"}]`),
					)
					reg.Register(
						httpmock.REST("GET", ""),
						httpmock.ScopesResponder("repo,read:org,read:public_key"),
					)
				},
				false,
				false,
				true,
				false,
			},
			want: []string{"✹ Name: Mac \n    SSH-KEY: ssh-rsa AAAABbBB123"},
		},
		{
			name: "config error",
			input: input{
				func(reg *httpmock.Registry) {
					reg.Register(
						httpmock.REST("GET", "user/keys"),
						httpmock.StringResponse(""),
					)
					reg.Register(
						httpmock.REST("GET", ""),
						httpmock.ScopesResponder("repo,read:org,read:public_key"),
					)
				},
				true,
				false,
				true,
				true,
			},
			want: []string{"X: Config error"},
		},
		{
			name: "http client error",
			input: input{
				func(reg *httpmock.Registry) {
					reg.Register(
						httpmock.REST("GET", "user/keys"),
						httpmock.StringResponse(""),
					)
					reg.Register(
						httpmock.REST("GET", ""),
						httpmock.ScopesResponder("repo,read:org,read:public_key"),
					)
				},
				false,
				true,
				true,
				true,
			},
			want: []string{"X: HttpClient error"},
		},
		{
			name: "not found on api.github.com/user/keys",
			input: input{
				func(reg *httpmock.Registry) {
					reg.Register(
						httpmock.REST("GET", "user/keys"),
						httpmock.StatusStringResponse(http.StatusNotFound, `{"message": "Not Found", "documentation_url": "url"}`),
					)
					reg.Register(
						httpmock.REST("GET", ""),
						httpmock.ScopesResponder("repo,read:org,read:public_key"),
					)
				},
				false,
				false,
				true,
				true,
			},
			want: []string{"X: Got HTTP 404 (https://api.github.com/user/keys)"},
		},
		{
			name: "missing scope",
			input: input{
				func(reg *httpmock.Registry) {
					reg.Register(
						httpmock.REST("GET", "user/keys"),
						httpmock.StringResponse(`[{"id":1234,"key":"ssh-rsa AAAABbBB123","title":"Mac"}]`),
					)
					reg.Register(
						httpmock.REST("GET", ""),
						httpmock.ScopesResponder(""),
					)
				},
				false,
				false,
				true,
				true,
			},
			want: []string{
				"X: missing required scope 'repo';missing required scope 'read:org';missing required scope 'read:public_key'",
				"- To request missing scopes, run: gh auth refresh -h github.com",
			},
		},
		{
			name: "authentication failed",
			input: input{
				func(reg *httpmock.Registry) {
					reg.Register(
						httpmock.REST("GET", "user/keys"),
						httpmock.StringResponse(`[{"id":1234,"key":"ssh-rsa AAAABbBB123","title":"Mac"}]`),
					)
					reg.Register(
						httpmock.REST("GET", ""),
						httpmock.StatusStringResponse(http.StatusNotFound, `{"message": "Not Found", "documentation_url": "url"}`),
					)
				},
				false,
				false,
				true,
				true,
			},
			want: []string{
				"X: authentication failed",
				"- The github.com token in ~/.config/gh/hosts.yml is no longer valid.",
				"- To re-authenticate, run: gh auth login -h github.com",
				"- To forget about this host, run: gh auth logout -h github.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			tt.input.httpStubs(reg)

			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(true)
			io.SetStdinTTY(true)
			io.SetStderrTTY(true)

			opts := ListOptions{
				HTTPClient: func() (*http.Client, error) {
					if tt.input.httpClientError {
						return nil, errors.New("HttpClient error")
					}
					return &http.Client{Transport: reg}, nil
				},
				IO: io,
				Config: func() (config.Config, error) {
					if tt.input.configError {
						return nil, errors.New("Config error")
					}
					cfg := config.NewBlankConfig()
					if tt.input.hasOauthToken {
						err := cfg.Set("github.com", "oauth_token", "abc123")
						if err != nil {
							return nil, err
						}
					}
					return cfg, nil
				},
			}

			err := listRun(&opts)
			if err != nil && !tt.input.wantErr {
				t.Errorf("linRun() return error: %v", err)
			}
			if !reflect.DeepEqual(opts.ListMsg, tt.want) {
				t.Errorf("linRun() = want %v, got %v", tt.want, opts.ListMsg)
			}
		})
	}
}
