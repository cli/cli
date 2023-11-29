package logout

import (
	"bytes"
	"io"
	"regexp"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/keyring"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdLogout(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants LogoutOptions
		tty   bool
	}{
		{
			name:  "nontty no arguments",
			cli:   "",
			wants: LogoutOptions{},
		},
		{
			name:  "tty no arguments",
			tty:   true,
			cli:   "",
			wants: LogoutOptions{},
		},
		{
			name: "tty with hostname",
			tty:  true,
			cli:  "--hostname github.com",
			wants: LogoutOptions{
				Hostname: "github.com",
			},
		},
		{
			name: "nontty with hostname",
			cli:  "--hostname github.com",
			wants: LogoutOptions{
				Hostname: "github.com",
			},
		},
		{
			name: "tty with user",
			tty:  true,
			cli:  "--user monalisa",
			wants: LogoutOptions{
				Username: "github.com",
			},
		},
		{
			name: "nontty with user",
			cli:  "--user monalisa",
			wants: LogoutOptions{
				Username: "github.com",
			},
		},
		{
			name: "tty with hostname and user",
			tty:  true,
			cli:  "--hostname github.com --user monalisa",
			wants: LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
		},
		{
			name: "nontty with hostname and user",
			cli:  "--hostname github.com --user monalisa",
			wants: LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)

			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)

			var gotOpts *LogoutOptions
			cmd := NewCmdLogout(f, func(opts *LogoutOptions) error {
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
			require.NoError(t, err)

			require.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
		})
	}
}

type host string
type user string

type hostUsers struct {
	host  host
	users []user
}

func Test_logoutRun_tty(t *testing.T) {
	tests := []struct {
		name          string
		opts          *LogoutOptions
		prompterStubs func(*prompter.PrompterMock)
		cfgHosts      []hostUsers
		secureStorage bool
		wantHosts     string
		wantErrOut    *regexp.Regexp
		wantErr       string
	}{
		{
			name: "logs out prompted user when multiple known hosts with one user each",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{"monalisa-ghe"}},
				{"github.com", []user{"monalisa"}},
			},
			wantHosts: "ghe.io:\n    users:\n        monalisa-ghe:\n            oauth_token: abc123\n            git_protocol: ssh\n    oauth_token: abc123\n    git_protocol: ssh\n    user: monalisa-ghe\n",
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "monalisa (github.com)")
				}
			},
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out prompted user when multiple known hosts with multiple users each",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{"monalisa-ghe", "monalisa-ghe2"}},
				{"github.com", []user{"monalisa", "monalisa2"}},
			},
			wantHosts: "ghe.io:\n    users:\n        monalisa-ghe:\n            oauth_token: abc123\n            git_protocol: ssh\n        monalisa-ghe2:\n            oauth_token: abc123\n            git_protocol: ssh\n    git_protocol: ssh\n    user: monalisa-ghe2\n    oauth_token: abc123\ngithub.com:\n    users:\n        monalisa2:\n            oauth_token: abc123\n            git_protocol: ssh\n    git_protocol: ssh\n    user: monalisa2\n    oauth_token: abc123\n",
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "monalisa (github.com)")
				}
			},
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out only logged in user",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
			},
			wantHosts:  "{}\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out prompted user when one known host with multiple users",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa", "monalisa2"}},
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "monalisa (github.com)")
				}
			},
			wantHosts:  "github.com:\n    users:\n        monalisa2:\n            oauth_token: abc123\n            git_protocol: ssh\n    git_protocol: ssh\n    user: monalisa2\n    oauth_token: abc123\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out specified user when multiple known hosts with one user each",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "monalisa-ghe",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{"monalisa-ghe"}},
				{"github.com", []user{"monalisa"}},
			},
			wantHosts:  "github.com:\n    users:\n        monalisa:\n            oauth_token: abc123\n            git_protocol: ssh\n    oauth_token: abc123\n    git_protocol: ssh\n    user: monalisa\n",
			wantErrOut: regexp.MustCompile(`Logged out of ghe.io account monalisa-ghe`),
		},
		{
			name:          "logs out specified user that is using secure storage",
			secureStorage: true,
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
			},
			wantHosts:  "{}\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name:    "errors when no known hosts",
			opts:    &LogoutOptions{},
			wantErr: `not logged in to any hosts`,
		},
		{
			name: "errors when specified host is not a known host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "monalisa-ghe",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
			},
			wantErr: "not logged in to ghe.io",
		},
		{
			name: "errors when specified user is not logged in on specified host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "unknown-user",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{"monalisa-ghe"}},
			},
			wantErr: "not logged in to ghe.io account unknown-user",
		},
		{
			name: "errors when user is specified but doesn't exist on any host",
			opts: &LogoutOptions{
				Username: "unknown-user",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
				{"ghe.io", []user{"monalisa"}},
			},
			wantErr: "no accounts matched that criteria",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			readConfigs := config.StubWriteConfig(t)
			cfg := config.NewFromString("")
			for _, hostUsers := range tt.cfgHosts {
				for _, user := range hostUsers.users {
					_, _ = cfg.Authentication().Login(
						string(hostUsers.host),
						string(user),
						"abc123", "ssh", tt.secureStorage,
					)
				}
			}

			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			err := logoutRun(tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantErrOut == nil {
				require.Equal(t, "", stderr.String())
			} else {
				require.True(t, tt.wantErrOut.MatchString(stderr.String()), stderr.String())
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)
			secureToken, _ := cfg.Authentication().TokenFromKeyring(tt.opts.Hostname)

			require.Equal(t, tt.wantHosts, hostsBuf.String())
			require.Equal(t, "", secureToken)
		})
	}
}

func Test_logoutRun_nontty(t *testing.T) {
	tests := []struct {
		name          string
		opts          *LogoutOptions
		cfgHosts      []hostUsers
		secureStorage bool
		ghtoken       string
		wantHosts     string
		wantErrOut    *regexp.Regexp
		wantErr       string
	}{
		{
			name: "logs out specified user when one known host",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
			},
			wantHosts:  "{}\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out specified user when multiple known hosts",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
				{"ghe.io", []user{"monalisa-ghe"}},
			},
			wantHosts:  "ghe.io:\n    users:\n        monalisa-ghe:\n            oauth_token: abc123\n            git_protocol: ssh\n    oauth_token: abc123\n    git_protocol: ssh\n    user: monalisa-ghe\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name:          "logs out specified user that is using secure storage",
			secureStorage: true,
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
			},
			wantHosts:  "{}\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "errors when no known hosts",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			wantErr: `not logged in to any hosts`,
		},
		{
			name: "errors when specified host is not a known host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "monalisa-ghe",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
			},
			wantErr: "not logged in to ghe.io",
		},
		{
			name: "errors when specified user is not logged in on specified host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "unknown-user",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{"monalisa-ghe"}},
			},
			wantErr: "not logged in to ghe.io account unknown-user",
		},
		{
			name: "errors when host is specified but user is ambiguous",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{"monalisa-ghe"}},
				{"ghe.io", []user{"monalisa-ghe-2"}},
			},
			wantErr: "unable to determine which account to log out of, please specify `--hostname` and `--user`",
		},
		{
			name: "errors when user is specified but host is ambiguous",
			opts: &LogoutOptions{
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{"monalisa"}},
				{"ghe.io", []user{"monalisa"}},
			},
			wantErr: "unable to determine which account to log out of, please specify `--hostname` and `--user`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			readConfigs := config.StubWriteConfig(t)
			cfg := config.NewFromString("")
			for _, hostUsers := range tt.cfgHosts {
				for _, user := range hostUsers.users {
					_, _ = cfg.Authentication().Login(
						string(hostUsers.host),
						string(user),
						"abc123", "ssh", tt.secureStorage,
					)
				}
			}
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(false)
			tt.opts.IO = ios

			err := logoutRun(tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantErrOut == nil {
				require.Equal(t, "", stderr.String())
			} else {
				require.True(t, tt.wantErrOut.MatchString(stderr.String()), stderr.String())
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)
			secureToken, _ := cfg.Authentication().TokenFromKeyring(tt.opts.Hostname)

			require.Equal(t, tt.wantHosts, hostsBuf.String())
			require.Equal(t, "", secureToken)
		})
	}
}

func TestLogoutSwitchesUserNonTTY(t *testing.T) {
	keyring.MockInit()

	ios, _, _, stderr := iostreams.Test()
	ios.SetStdinTTY(false)
	ios.SetStdoutTTY(false)

	readConfigs := config.StubWriteConfig(t)
	cfg := config.NewFromString("")
	_, err := cfg.Authentication().Login("github.com", "test-user-1", "test-token-1", "https", true)
	require.NoError(t, err)

	_, err = cfg.Authentication().Login("github.com", "test-user-2", "test-token-2", "ssh", true)
	require.NoError(t, err)

	opts := LogoutOptions{
		IO: ios,
		Config: func() (config.Config, error) {
			return cfg, nil
		},
		Hostname: "github.com",
		Username: "test-user-2",
	}

	require.NoError(t, logoutRun(&opts))

	hostsBuf := bytes.Buffer{}
	readConfigs(io.Discard, &hostsBuf)

	secureToken, _ := cfg.Authentication().TokenFromKeyring("github.com")
	require.Equal(t, "test-token-1", secureToken)

	expectedHosts := heredoc.Doc(`
        github.com:
            users:
                test-user-1:
                    git_protocol: https
            git_protocol: https
            user: test-user-1
    `)

	require.Equal(t, expectedHosts, hostsBuf.String())

	require.Contains(t, stderr.String(), "✓ Switched active account for github.com to test-user-1")
}

func TestLogoutSwitchesUserTTY(t *testing.T) {
	keyring.MockInit()

	ios, _, _, stderr := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	readConfigs := config.StubWriteConfig(t)
	cfg := config.NewFromString("")
	_, err := cfg.Authentication().Login("github.com", "test-user-1", "test-token-1", "https", true)
	require.NoError(t, err)

	_, err = cfg.Authentication().Login("github.com", "test-user-2", "test-token-2", "ssh", true)
	require.NoError(t, err)

	pm := &prompter.PrompterMock{}
	pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
		return prompter.IndexFor(opts, "test-user-2 (github.com)")
	}

	opts := LogoutOptions{
		IO: ios,
		Config: func() (config.Config, error) {
			return cfg, nil
		},
		Prompter: pm,
	}

	require.NoError(t, logoutRun(&opts))

	hostsBuf := bytes.Buffer{}
	readConfigs(io.Discard, &hostsBuf)

	secureToken, _ := cfg.Authentication().TokenFromKeyring("github.com")
	require.Equal(t, "test-token-1", secureToken)

	expectedHosts := heredoc.Doc(`
        github.com:
            users:
                test-user-1:
                    git_protocol: https
            git_protocol: https
            user: test-user-1
    `)

	require.Equal(t, expectedHosts, hostsBuf.String())

	require.Contains(t, stderr.String(), "✓ Switched active account for github.com to test-user-1")
}
