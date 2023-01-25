package status

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
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
		name      string
		opts      *StatusOptions
		httpStubs func(*httpmock.Registry)
		cfgStubs  func(*config.ConfigMock)
		wantErr   string
		wantOut   string
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
				// mocks for HeaderHasMinimumScopes api requests to a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantOut: heredoc.Doc(`
				joel.miller
				  ✓ Logged in to joel.miller as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for joel.miller configured to use https protocol.
				  ✓ Token: ******
				  ✓ Token scopes: repo,read:org
			`),
		},
		{
			name: "missing scope",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("joel.miller", "oauth_token", "abc123")
				c.Set("github.com", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HeaderHasMinimumScopes api requests to a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo"))
				// mocks for HeaderHasMinimumScopes api requests to github.com host
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantErr: "SilentError",
			wantOut: heredoc.Doc(`
				joel.miller
				  X joel.miller: the token in GH_CONFIG_DIR/hosts.yml is missing required scope 'read:org'
				  - To request missing scopes, run: gh auth refresh -h joel.miller
				
				github.com
				  ✓ Logged in to github.com as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for github.com configured to use https protocol.
				  ✓ Token: ******
				  ✓ Token scopes: repo,read:org
			`),
		},
		{
			name: "bad token",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("joel.miller", "oauth_token", "abc123")
				c.Set("github.com", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mock for HeaderHasMinimumScopes api requests to a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.StatusStringResponse(400, "no bueno"))
				// mock for HeaderHasMinimumScopes api requests to github.com
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantErr: "SilentError",
			wantOut: heredoc.Doc(`
				joel.miller
				  X joel.miller: authentication failed
				  - The joel.miller token in GH_CONFIG_DIR/hosts.yml is no longer valid.
				  - To re-authenticate, run: gh auth login -h joel.miller
				  - To forget about this host, run: gh auth logout -h joel.miller
				
				github.com
				  ✓ Logged in to github.com as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for github.com configured to use https protocol.
				  ✓ Token: ******
				  ✓ Token scopes: repo,read:org
			`),
		},
		{
			name: "all good",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "gho_abc123")
				c.Set("joel.miller", "oauth_token", "gho_abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HeaderHasMinimumScopes api requests to github.com
				reg.Register(
					httpmock.REST("GET", ""),
					httpmock.WithHeader(httpmock.ScopesResponder("repo,read:org"), "X-Oauth-Scopes", "repo, read:org"))
				// mocks for HeaderHasMinimumScopes api requests to a non-github.com host
				reg.Register(
					httpmock.REST("GET", "api/v3/"),
					httpmock.WithHeader(httpmock.ScopesResponder("repo,read:org"), "X-Oauth-Scopes", ""))
				// mock for CurrentLoginName, one for each host
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantOut: heredoc.Doc(`
				github.com
				  ✓ Logged in to github.com as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for github.com configured to use https protocol.
				  ✓ Token: gho_******
				  ✓ Token scopes: repo, read:org
				
				joel.miller
				  ✓ Logged in to joel.miller as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for joel.miller configured to use https protocol.
				  ✓ Token: gho_******
				  X Token scopes: none
			`),
		},
		{
			name: "server-to-server token",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "ghs_xxx")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HeaderHasMinimumScopes api requests to github.com
				reg.Register(
					httpmock.REST("GET", ""),
					httpmock.ScopesResponder(""))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantOut: heredoc.Doc(`
				github.com
				  ✓ Logged in to github.com as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for github.com configured to use https protocol.
				  ✓ Token: ghs_***
			`),
		},
		{
			name: "PAT V2 token",
			opts: &StatusOptions{},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "github_pat_xxx")
			},
			httpStubs: func(reg *httpmock.Registry) {
				// mocks for HeaderHasMinimumScopes api requests to github.com
				reg.Register(
					httpmock.REST("GET", ""),
					httpmock.ScopesResponder(""))
				// mock for CurrentLoginName
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantOut: heredoc.Doc(`
				github.com
				  ✓ Logged in to github.com as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for github.com configured to use https protocol.
				  ✓ Token: github_pat_***
			`),
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
				// mocks for HeaderHasMinimumScopes on a non-github.com host
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				// mocks for HeaderHasMinimumScopes on github.com
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// mock for CurrentLoginName, one for each host
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"tess"}}}`))
			},
			wantOut: heredoc.Doc(`
				github.com
				  ✓ Logged in to github.com as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for github.com configured to use https protocol.
				  ✓ Token: xyz456
				  ✓ Token scopes: repo,read:org
				
				joel.miller
				  ✓ Logged in to joel.miller as tess (GH_CONFIG_DIR/hosts.yml)
				  ✓ Git operations for joel.miller configured to use https protocol.
				  ✓ Token: abc123
				  ✓ Token scopes: repo,read:org
			`),
		},
		{
			name: "missing hostname",
			opts: &StatusOptions{
				Hostname: "github.example.com",
			},
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "abc123")
			},
			httpStubs: func(reg *httpmock.Registry) {},
			wantErr:   "SilentError",
			wantOut:   "Hostname \"github.example.com\" not found among authenticated GitHub hosts\n",
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
			defer reg.Verify(t)
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			err := statusRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			output := strings.ReplaceAll(stderr.String(), config.ConfigDir()+string(filepath.Separator), "GH_CONFIG_DIR/")
			assert.Equal(t, tt.wantOut, output)

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)

			assert.Equal(t, "", mainBuf.String())
			assert.Equal(t, "", hostsBuf.String())
		})
	}
}
