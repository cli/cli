package status

import (
	"bytes"
	"net/http"
	"regexp"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func Test_NewCmdStatus(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants StatusOptions
	}{
		{
			name:  "no arguments",
			cli:   "",
			wants: StatusOptions{},
		},
		{
			name: "hostname set",
			cli:  "--hostname ellie.williams",
			wants: StatusOptions{
				Hostname: "ellie.williams",
			},
		},
		{
			name: "show token",
			cli:  "--show-token",
			wants: StatusOptions{
				ShowToken: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *StatusOptions
			cmd := NewCmdStatus(f, func(opts *StatusOptions) error {
				gotOpts = opts
				return nil
			})

			// TODO cobra hack-around
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
		})
	}
}

func Test_statusRun(t *testing.T) {
	readConfigs := config.StubWriteConfig(t)

	tests := []struct {
		name       string
		opts       *StatusOptions
		httpStubs  func(*httpmock.Registry)
		cfgStubs   func(*config.ConfigMock)
		wantErr    string
		wantErrOut *regexp.Regexp
	}{
		{
			name: "hostname set",
			opts: &StatusOptions{
				Hostname: "joel.miller",
			},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("joel.miller", "oauth_token", "abc123")
				c.Set("github.com", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HasMinimumScopes and GetScopes api requests to a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantErrOut: regexp.MustCompile(`Logged in to joel.miller as.*tess`),
		},
		{
			name: "missing scope",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("joel.miller", "oauth_token", "abc123")
				c.Set("github.com", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HasMinimumScopes and GetScopes api requests to a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo"))
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				// mocks for HasMinimumScopes api requests to github.com host
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantErrOut: regexp.MustCompile(`joel.miller: missing required.*Logged in to github.com as.*tess`),
			wantErr:    "SilentError",
		},
		{
			name: "bad token",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("joel.miller", "oauth_token", "abc123")
				c.Set("github.com", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mock for HasMinimumScopes api requests to a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.StatusStringResponse(400, "no bueno"))
				// mock for GetScopes api requests to github.com
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantErrOut: regexp.MustCompile(`joel.miller: authentication failed.*Logged in to github.com as.*tess`),
			wantErr:    "SilentError",
		},
		{
			name: "all good",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "abc123")
				c.Set("joel.miller", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HasMinimumScopes and GetScopes api requests to a non-github.com host
				// the second one unsets the scopes header
				reg.Register(
					httpmock.REST("GET", "api/v3/"),
					httpmock.WithHeader(httpmock.ScopesResponder("repo,read:org"), "X-Oauth-Scopes", "repo, read:org"))
				reg.Register(
					httpmock.REST("GET", "api/v3/"),
					httpmock.WithHeader(httpmock.ScopesResponder("repo,read:org"), "X-Oauth-Scopes", ""))
				// mocks for HasMinimumScopes and GetScopes api requests to github.com
				reg.Register(
					httpmock.REST("GET", ""),
					httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.REST("GET", ""),
					httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName, one for each host
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantErrOut: regexp.MustCompile(`(?s)Logged in to github.com as.*tess.*Token Scopes: repo,read:org.*Logged in to joel.miller as.*tess.*X Token Scopes: None found`),
		},
		{
			name: "hide token",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("joel.miller", "oauth_token", "abc123")
				c.Set("github.com", "oauth_token", "xyz456")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HasMinimumScopes and GetScopes api requests to a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				// mocks for HasMinimumScopes and GetScopes api requests to github.com
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName, one for each host
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))

			},
			wantErrOut: regexp.MustCompile(`(?s)Token: \*{19}.*Token: \*{19}`),
		},
		{
			name: "show token",
			opts: &StatusOptions{
				ShowToken: true,
			},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "xyz456")
				c.Set("joel.miller", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HasMinimumScopes and GetScopes on a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				// mocks for HasMinimumScopes and GetScopes on github.com
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName, one for each host
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantErrOut: regexp.MustCompile(`(?s)Token: xyz456.*Token: abc123`),
		},
		{
			name: "missing hostname",
			opts: &StatusOptions{
				Hostname: "github.example.com",
			},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "abc123")
			},
			httpStubs:  func(reg *httpmock.Registry) {},
			wantErrOut: regexp.MustCompile(`(?s)Hostname "github.example.com" not found among authenticated GitHub hosts`),
			wantErr:    "SilentError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts == nil {
				tt.opts = &StatusOptions{}
			}

			ios, _, _, stderr := iostreams.Test()

			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			cfg := config.NewFromString("")
			if tt.cfgStubs != nil {
				tt.cfgStubs(cfg)
			}
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			reg := &httpmock.Registry{}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			err := statusRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			} else {
				assert.NoError(t, err)
			}

			if tt.wantErrOut == nil {
				assert.Equal(t, "", stderr.String())
			} else {
				assert.True(t, tt.wantErrOut.MatchString(stderr.String()))
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)

			assert.Equal(t, "", mainBuf.String())
			assert.Equal(t, "", hostsBuf.String())

			reg.Verify(t)
		})
	}
}
