package refresh

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

// TODO prompt cfg test

func Test_NewCmdRefresh(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       RefreshOptions
		wantsErr    bool
		tty         bool
		neverPrompt bool
	}{
		{
			name: "tty no arguments",
			tty:  true,
			wants: RefreshOptions{
				Hostname: "",
			},
		},
		{
			name:     "nontty no arguments",
			wantsErr: true,
		},
		{
			name: "nontty hostname",
			cli:  "-h aline.cedrac",
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name: "tty hostname",
			tty:  true,
			cli:  "-h aline.cedrac",
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name:        "prompts disabled, no args",
			tty:         true,
			cli:         "",
			neverPrompt: true,
			wantsErr:    true,
		},
		{
			name:        "prompts disabled, hostname",
			tty:         true,
			cli:         "-h aline.cedrac",
			neverPrompt: true,
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name: "tty one scope",
			tty:  true,
			cli:  "--scopes repo:invite",
			wants: RefreshOptions{
				Scopes: []string{"repo:invite"},
			},
		},
		{
			name: "tty scopes",
			tty:  true,
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
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)
			io.SetNeverPrompt(tt.neverPrompt)

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
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
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
		wantErr      string
		nontty       bool
		wantAuthArgs authArgs
	}{
		{
			name:    "no hosts configured",
			opts:    &RefreshOptions{},
			wantErr: `not logged in to any hosts`,
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
			wantErr: `not logged in to obed.morton`,
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
			tt.opts.AuthFlow = func(_ config.Config, _ *iostreams.IOStreams, hostname string, scopes []string) error {
				aa.hostname = hostname
				aa.scopes = scopes
				return nil
			}

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
			if tt.wantErr != "" {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, aa.hostname, tt.wantAuthArgs.hostname)
			assert.Equal(t, aa.scopes, tt.wantAuthArgs.scopes)
		})
	}
}
