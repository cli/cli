package root

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestConfigIndependentCommands(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"version", "--help"}, {"issue", "list", "--help"}, {"help", "environment"}, {"completion", "-s", "bash"}, {"completion", "-s", "fish"}, {"completion", "-s", "zsh"}, {"completion", "-s", "powershell"}} {
		t.Run(args[0], func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			loads := 0
			f := &cmdutil.Factory{
				IOStreams: ios,
				Browser: &browser.Stub{},
				Config: func() (gh.Config, error) {
					loads++
					return nil, errors.New("unreadable config")
				},
			}
			cmd, err := NewCmdRoot(f, "v1.2.3", "", args)
			require.NoError(t, err)
			require.NoError(t, cmd.Execute())
			require.NotEmpty(t, stdout.String())
			require.Zero(t, loads)
		})
	}
}

func TestConfigBackedDispatch(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		config     string
		configErr  error
		wantErr    string
		wantOutput string
	}{
		{name: "alias", args: []string{"v"}, config: "aliases:\n  v: version\n", wantOutput: "gh version 1.2.3"},
		{name: "completion aliases", args: []string{"__complete", "v"}, config: "aliases:\n  v: version\n", wantOutput: "Alias for"},
		{name: "root help aliases", args: []string{"--help"}, config: "aliases:\n  v: version\n", wantOutput: "v:"},
		{name: "version flag disabled", args: []string{"--version", "--version=false"}, config: "aliases:\n  v: version\n", wantOutput: "v:"},
		{name: "parent help aliases", args: []string{"repo", "--help"}, config: "aliases:\n  repo v: version\n", wantOutput: "v:"},
		{name: "alias help", args: []string{"help", "v"}, config: "aliases:\n  v: version\n", wantOutput: "gh version"},
		{name: "shell alias", args: []string{"s"}, config: "aliases:\n  s: '!printf shell-alias-output'\n", wantOutput: "shell-alias-output"},
		{name: "nested alias", args: []string{"repo", "v"}, config: "aliases:\n  repo v: version\n", wantOutput: "gh version 1.2.3"},
		{name: "auth required", args: []string{"issue", "list"}, wantErr: "auth"},
		{name: "auth config error", args: []string{"issue", "list"}, configErr: errors.New("malformed config"), wantErr: "malformed config"},
		{name: "alias config error", args: []string{"v"}, configErr: errors.New("unreadable config"), wantErr: "unreadable config"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_TOKEN", "")
			t.Setenv("GITHUB_TOKEN", "")
			ios, _, stdout, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Browser: &browser.Stub{}, Config: func() (gh.Config, error) {
				return config.NewMockConfigFromString(tt.config), tt.configErr
			}}
			cmd, err := NewCmdRoot(f, "v1.2.3", "", tt.args)
			if err == nil {
				cmd.SetOut(ios.Out)
				cmd.SetErr(ios.ErrOut)
				err = cmd.Execute()
			}
			if tt.wantErr == "auth" {
				var authErr *AuthError
				require.ErrorAs(t, err, &authErr)
			} else if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorIs(t, err, tt.configErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, stdout.String(), tt.wantOutput)
			}
		})
	}
}

func TestConfigBackedAuthentication(t *testing.T) {
	for _, tt := range []struct {
		name   string
		token  string
		config string
	}{
		{name: "environment token", token: "env-token"},
		{name: "configured host", config: "hosts:\n  github.com:\n    oauth_token: config-token\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := config.NewIsolatedTestConfig(t, tt.config)
			t.Setenv("GH_TOKEN", tt.token)
			t.Setenv("GITHUB_TOKEN", "")
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios, Browser: &browser.Stub{}, Config: func() (gh.Config, error) {
				return cfg, nil
			}}
			cmd, err := NewCmdRoot(f, "v1.2.3", "", []string{"issue", "list"})
			require.NoError(t, err)
			selected, _, err := cmd.Find([]string{"issue", "list"})
			require.NoError(t, err)
			ran := false
			selected.RunE = func(*cobra.Command, []string) error {
				ran = true
				return nil
			}
			require.NoError(t, cmd.Execute())
			require.True(t, ran)
		})
	}
}
