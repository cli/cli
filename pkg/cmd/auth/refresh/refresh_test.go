package refresh

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func Test_NewCmdRefresh(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants RefreshOptions
	}{
		{
			name: "no arguments",
			wants: RefreshOptions{
				Hostname: "",
			},
		},
		{
			name: "hostname",
			cli:  "-h aline.cedrac",
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name: "one scope",
			cli:  "--scopes repo:invite",
			wants: RefreshOptions{
				Scopes: []string{"repo:invite"},
			},
		},
		{
			name: "scopes",
			cli:  "--scopes repo:invite,read:public_key",
			wants: RefreshOptions{
				Scopes: []string{"repo:invite", "read:public_key"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *RefreshOptions
			cmd := NewCmdRefresh(f, func(opts *RefreshOptions) error {
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
			assert.Equal(t, tt.wants.Scopes, gotOpts.Scopes)
		})

	}
}

type authArgs struct {
	hostname string
	scopes   []string
}

func Test_refreshRun(t *testing.T) {
	tests := []struct {
		name         string
		opts         *RefreshOptions
		askStubs     func(*prompt.AskStubber)
		cfgHosts     []string
		wantErr      *regexp.Regexp
		ghtoken      string
		nontty       bool
		wantAuthArgs authArgs
	}{
		{
			name:    "GITHUB_TOKEN set",
			opts:    &RefreshOptions{},
			ghtoken: "abc123",
			wantErr: regexp.MustCompile(`GITHUB_TOKEN is present in your environment`),
		},
		{
			name:    "non tty",
			opts:    &RefreshOptions{},
			nontty:  true,
			wantErr: regexp.MustCompile(`not attached to a terminal;`),
		},
		{
			name:    "no hosts configured",
			opts:    &RefreshOptions{},
			wantErr: regexp.MustCompile(`not logged in to any hosts`),
		},
		{
			name: "hostname given but dne",
			cfgHosts: []string{
				"github.com",
				"aline.cedrac",
			},
			opts: &RefreshOptions{
				Hostname: "obed.morton",
			},
			wantErr: regexp.MustCompile(`not logged in to obed.morton`),
		},
		{
			name: "hostname provided and is configured",
			cfgHosts: []string{
				"obed.morton",
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname: "obed.morton",
			},
			wantAuthArgs: authArgs{
				hostname: "obed.morton",
				scopes:   nil,
			},
		},
		{
			name: "no hostname, one host configured",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname: "",
			},
			wantAuthArgs: authArgs{
				hostname: "github.com",
				scopes:   nil,
			},
		},
		{
			name: "no hostname, multiple hosts configured",
			cfgHosts: []string{
				"github.com",
				"aline.cedrac",
			},
			opts: &RefreshOptions{
				Hostname: "",
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne("github.com")
			},
			wantAuthArgs: authArgs{
				hostname: "github.com",
				scopes:   nil,
			},
		},
		{
			name: "scopes provided",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Scopes: []string{"repo:invite", "public_key:read"},
			},
			wantAuthArgs: authArgs{
				hostname: "github.com",
				scopes:   []string{"repo:invite", "public_key:read"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aa := authArgs{}
			tt.opts.AuthFlow = func(_ config.Config, hostname string, scopes []string) error {
				aa.hostname = hostname
				aa.scopes = scopes
				return nil
			}

			ghtoken := os.Getenv("GITHUB_TOKEN")
			defer func() {
				os.Setenv("GITHUB_TOKEN", ghtoken)
			}()
			os.Setenv("GITHUB_TOKEN", tt.ghtoken)
			io, _, _, _ := iostreams.Test()

			io.SetStdinTTY(!tt.nontty)
			io.SetStdoutTTY(!tt.nontty)

			tt.opts.IO = io
			cfg := config.NewBlankConfig()
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}
			for _, hostname := range tt.cfgHosts {
				_ = cfg.Set(hostname, "oauth_token", "abc123")
			}
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.GraphQL(`query UserCurrent\b`),
				httpmock.StringResponse(`{"data":{"viewer":{"login":"cybilb"}}}`))

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			as, teardown := prompt.InitAskStubber()
			defer teardown()
			if tt.askStubs != nil {
				tt.askStubs(as)
			}

			err := refreshRun(tt.opts)
			assert.Equal(t, tt.wantErr == nil, err == nil)
			if err != nil {
				if tt.wantErr != nil {
					assert.True(t, tt.wantErr.MatchString(err.Error()))
					return
				} else {
					t.Fatalf("unexpected error: %s", err)
				}
			}

			assert.Equal(t, aa.hostname, tt.wantAuthArgs.hostname)
			assert.Equal(t, aa.scopes, tt.wantAuthArgs.scopes)
		})
	}
}
