package authswitch

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/keyring"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestNewCmdSwitch(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOpts   SwitchOptions
		expectedErrMsg string
	}{
		{
			name:         "no flags",
			input:        "",
			expectedOpts: SwitchOptions{},
		},
		{
			name:  "hostname flag",
			input: "--hostname github.com",
			expectedOpts: SwitchOptions{
				Hostname: "github.com",
			},
		},
		{
			name:  "user flag",
			input: "--user monalisa",
			expectedOpts: SwitchOptions{
				User: "monalisa",
			},
		},
		{
			name:           "positional args is an error",
			input:          "some-positional-arg",
			expectedErrMsg: "accepts 0 arg(s), received 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}
			argv, err := shlex.Split(tt.input)
			require.NoError(t, err)

			var gotOpts *SwitchOptions
			cmd := NewCmdSwitch(f, func(opts *SwitchOptions) error {
				gotOpts = opts
				return nil
			})
			// Override the help flag as happens in production to allow -h flag
			// to be used for hostname.
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.expectedErrMsg != "" {
				require.ErrorContains(t, err, tt.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			require.Equal(t, &tt.expectedOpts, gotOpts)
		})
	}

}

func TestSwitchRun(t *testing.T) {
	type host string
	type user struct {
		name  string
		token string
	}

	type hostUsers struct {
		host  host
		users []user
	}

	tests := []struct {
		name                 string
		opts                 SwitchOptions
		cfgHosts             []hostUsers
		env                  map[string]string
		expectedHostToSwitch string
		expectedActiveUser   string
		expectedActiveToken  string
		expectedHosts        string
		expectedStderr       string
		expectedErr          error

		prompterStubs func(*prompter.PrompterMock)
	}{
		{
			name: "given one host with two users, switches to the other user",
			opts: SwitchOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
			},
			expectedHostToSwitch: "github.com",
			expectedActiveUser:   "inactive-user",
			expectedActiveToken:  "inactive-user-token",
			expectedHosts:        "github.com:\n    users:\n        inactive-user:\n            git_protocol: ssh\n        active-user:\n            git_protocol: ssh\n    git_protocol: ssh\n    user: inactive-user\n",
			expectedStderr:       "✓ Switched active account on github.com to 'inactive-user'",
		},
		{
			name: "given one host, with three users, switches to the specified user",
			opts: SwitchOptions{
				User: "inactive-user-2",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user-1", "inactive-user-1-token"},
					{"inactive-user-2", "inactive-user-2-token"},
					{"active-user", "active-user-token"},
				}},
			},
			expectedHostToSwitch: "github.com",
			expectedActiveUser:   "inactive-user-2",
			expectedActiveToken:  "inactive-user-2-token",
			expectedHosts:        "github.com:\n    users:\n        inactive-user-1:\n            git_protocol: ssh\n        inactive-user-2:\n            git_protocol: ssh\n        active-user:\n            git_protocol: ssh\n    git_protocol: ssh\n    user: inactive-user-2\n",
			expectedStderr:       "✓ Switched active account on github.com to 'inactive-user-2'",
		},
		{
			name: "given multiple hosts, with multiple users, switches to the specific user on the host",
			opts: SwitchOptions{
				Hostname: "ghe.io",
				User:     "inactive-user",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
				{"ghe.io", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
			},
			expectedHostToSwitch: "ghe.io",
			expectedActiveUser:   "inactive-user",
			expectedActiveToken:  "inactive-user-token",
			expectedHosts:        "github.com:\n    users:\n        inactive-user:\n            git_protocol: ssh\n        active-user:\n            git_protocol: ssh\n    git_protocol: ssh\n    user: active-user\nghe.io:\n    users:\n        inactive-user:\n            git_protocol: ssh\n        active-user:\n            git_protocol: ssh\n    git_protocol: ssh\n    user: inactive-user\n",
			expectedStderr:       "✓ Switched active account on ghe.io to 'inactive-user'",
		},
		{
			name:        "given we're not logged into any hosts, provide an informative error",
			opts:        SwitchOptions{},
			cfgHosts:    []hostUsers{},
			expectedErr: fmt.Errorf("not logged in to any hosts"),
		},
		{
			name: "given we can't disambiguate users across hosts",
			opts: SwitchOptions{
				User: "inactive-user",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
				{"ghe.io", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
			},
			expectedErr: errors.New("unable to determine which user account to switch to, please specify `--hostname` and `--user`"),
		},
		{
			name: "given we can't disambiguate user on a single host",
			opts: SwitchOptions{
				Hostname: "github.com",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user-1", "inactive-user-1-token"},
					{"inactive-user-2", "inactive-user-2-token"},
					{"active-user", "active-user-token"},
				}},
			},
			expectedErr: errors.New("unable to determine which user account to switch to, please specify `--hostname` and `--user`"),
		},
		{
			name: "given the auth token isn't writeable (e.g. a token env var is set)",
			opts: SwitchOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
			},
			env:            map[string]string{"GH_TOKEN": "unimportant-test-value"},
			expectedErr:    cmdutil.SilentError,
			expectedStderr: "The value of the GH_TOKEN environment variable is being used for authentication.",
		},
		{
			name: "specified hostname doesn't exist",
			opts: SwitchOptions{
				Hostname: "ghe.io",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
			},
			expectedErr: errors.New("not logged in to ghe.io"),
		},
		{
			name: "specified user doesn't exist on host",
			opts: SwitchOptions{
				Hostname: "github.com",
				User:     "non-existent-user",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
			},
			expectedErr: errors.New("not logged in as non-existent-user on github.com"),
		},
		{
			name: "specified user doesn't exist on any host",
			opts: SwitchOptions{
				User: "non-existent-user",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"active-user", "active-user-token"},
				}},
				{"ghe.io", []user{
					{"active-user", "active-user-token"},
				}},
			},
			expectedErr: errors.New("no user accounts matched that criteria"),
		},
		{
			name: "when options need to be disambiguated, the user is prompted with matrix of options including active users (if possible)",
			opts: SwitchOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
				{"ghe.io", []user{
					{"inactive-user", "inactive-user-token"},
					{"active-user", "active-user-token"},
				}},
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					require.Equal(t, "What account do you want to switch to?", prompt)
					require.Equal(t, []string{
						"inactive-user (github.com)",
						"active-user (github.com) - active",
						"inactive-user (ghe.io)",
						"active-user (ghe.io) - active",
					}, opts)

					return prompter.IndexFor(opts, "inactive-user (ghe.io)")
				}
			},
			expectedHostToSwitch: "ghe.io",
			expectedActiveUser:   "inactive-user",
			expectedActiveToken:  "inactive-user-token",
			expectedHosts:        "github.com:\n    users:\n        inactive-user:\n            git_protocol: ssh\n        active-user:\n            git_protocol: ssh\n    git_protocol: ssh\n    user: active-user\nghe.io:\n    users:\n        inactive-user:\n            git_protocol: ssh\n        active-user:\n            git_protocol: ssh\n    git_protocol: ssh\n    user: inactive-user\n",
			expectedStderr:       "✓ Switched active account on ghe.io to 'inactive-user'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()

			cfg, readConfigs := config.NewIsolatedTestConfig(t)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			isInteractive := tt.prompterStubs != nil
			if isInteractive {
				pm := &prompter.PrompterMock{}
				tt.prompterStubs(pm)
				tt.opts.Prompter = pm
			}

			for _, hostUsers := range tt.cfgHosts {
				for _, user := range hostUsers.users {
					_, err := cfg.Authentication().Login(
						string(hostUsers.host),
						user.name,
						user.token, "ssh", true,
					)
					require.NoError(t, err)
				}
			}

			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(isInteractive)
			ios.SetStdoutTTY(isInteractive)
			tt.opts.IO = ios

			err := switchRun(&tt.opts)
			if tt.expectedErr != nil {
				require.Equal(t, tt.expectedErr, err)
				require.Contains(t, stderr.String(), tt.expectedStderr)
				return
			}

			require.NoError(t, err)

			activeUser, err := cfg.Authentication().User(tt.expectedHostToSwitch)
			require.NoError(t, err)
			require.Equal(t, tt.expectedActiveUser, activeUser)

			activeToken, _ := cfg.Authentication().TokenFromKeyring(tt.expectedHostToSwitch)
			require.Equal(t, tt.expectedActiveToken, activeToken)

			hostsBuf := bytes.Buffer{}
			readConfigs(io.Discard, &hostsBuf)

			require.Equal(t, tt.expectedHosts, hostsBuf.String())

			require.Contains(t, stderr.String(), tt.expectedStderr)
		})
	}
}
