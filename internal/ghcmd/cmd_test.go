package ghcmd

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/agents"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_printError(t *testing.T) {
	rootCmd := &cobra.Command{Use: "gh"}
	cmd := &cobra.Command{
		Use:   "spend",
		Short: "Spend money",
		Example: heredoc.Doc(`
			$ gh spend --amount 1
		`),
	}
	cmd.Flags().Int("amount", 0, "How much to spend")
	rootCmd.AddCommand(cmd)

	type args struct {
		err      error
		cmd      *cobra.Command
		debug    bool
		fullHelp bool
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		{
			name: "generic error",
			args: args{
				err:   errors.New("the app exploded"),
				cmd:   nil,
				debug: false,
			},
			wantOut: "the app exploded\n",
		},
		{
			name: "DNS error",
			args: args{
				err: fmt.Errorf("DNS oopsie: %w", &net.DNSError{
					Name: "api.github.com",
				}),
				cmd:   nil,
				debug: false,
			},
			wantOut: `error connecting to api.github.com
check your internet connection or https://githubstatus.com
`,
		},
		{
			name: "Cobra flag error",
			args: args{
				err:   cmdutil.FlagErrorf("unknown flag --foo"),
				cmd:   cmd,
				debug: false,
			},
			wantOut: "unknown flag --foo\n\n" + cmd.UsageString() + "\n",
		},
		{
			name: "unknown Cobra command error",
			args: args{
				err:   errors.New("unknown command foo"),
				cmd:   cmd,
				debug: false,
			},
			wantOut: "unknown command foo\n\n" + cmd.UsageString() + "\n",
		},
		{
			name: "Cobra flag error with full help",
			args: args{
				err:      cmdutil.FlagErrorf("unknown flag --foo"),
				cmd:      cmd,
				debug:    false,
				fullHelp: true,
			},
			wantOut: heredoc.Doc(`
				unknown flag --foo

				Spend money

				USAGE
				  gh spend [flags]

				FLAGS
				  --amount int   How much to spend

				EXAMPLES
				  $ gh spend --amount 1

				LEARN MORE
				  Use ` + "`gh <command> <subcommand> --help`" + ` for more information about a command.
				  Read the manual at https://cli.github.com/manual
				  Learn about exit codes using ` + "`gh help exit-codes`" + `
				  Learn about accessibility experiences using ` + "`gh help accessibility`" + `

			`),
		},
		{
			name: "unknown Cobra command error with full help",
			args: args{
				err:      errors.New("unknown command foo"),
				cmd:      cmd,
				debug:    false,
				fullHelp: true,
			},
			wantOut: heredoc.Doc(`
				unknown command foo

				Spend money

				USAGE
				  gh spend [flags]

				FLAGS
				  --amount int   How much to spend

				EXAMPLES
				  $ gh spend --amount 1

				LEARN MORE
				  Use ` + "`gh <command> <subcommand> --help`" + ` for more information about a command.
				  Read the manual at https://cli.github.com/manual
				  Learn about exit codes using ` + "`gh help exit-codes`" + `
				  Learn about accessibility experiences using ` + "`gh help accessibility`" + `

			`),
		},
		{
			name: "generic error is unaffected by full help",
			args: args{
				err:      errors.New("the app exploded"),
				cmd:      cmd,
				debug:    false,
				fullHelp: true,
			},
			wantOut: "the app exploded\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			out := &bytes.Buffer{}
			printError(out, ios.ColorScheme(), tt.args.err, tt.args.cmd, tt.args.debug, tt.args.fullHelp)
			assert.Equal(t, tt.wantOut, out.String())
		})
	}
}

func Test_newIOStreams_pager(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		config    gh.Config
		wantPager string
	}{
		{
			name: "GH_PAGER and PAGER set",
			env: map[string]string{
				"GH_PAGER": "GH_PAGER",
				"PAGER":    "PAGER",
			},
			wantPager: "GH_PAGER",
		},
		{
			name: "GH_PAGER and config pager set",
			env: map[string]string{
				"GH_PAGER": "GH_PAGER",
			},
			config:    pagerConfig(),
			wantPager: "GH_PAGER",
		},
		{
			name: "config pager and PAGER set",
			env: map[string]string{
				"PAGER": "PAGER",
			},
			config:    pagerConfig(),
			wantPager: "CONFIG_PAGER",
		},
		{
			name: "only PAGER set",
			env: map[string]string{
				"PAGER": "PAGER",
			},
			wantPager: "PAGER",
		},
		{
			name: "GH_PAGER set to blank string",
			env: map[string]string{
				"GH_PAGER": "",
				"PAGER":    "PAGER",
			},
			wantPager: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					t.Setenv(k, v)
				}
			}
			var cfg gh.Config
			if tt.config != nil {
				cfg = tt.config
			} else {
				cfg = config.NewMockConfig()
			}
			io := newIOStreams(cfg, "")
			assert.Equal(t, tt.wantPager, io.GetPager())
		})
	}
}

func Test_newIOStreams_prompt(t *testing.T) {
	tests := []struct {
		name           string
		config         gh.Config
		promptDisabled bool
		env            map[string]string
	}{
		{
			name:           "default config",
			promptDisabled: false,
		},
		{
			name:           "config with prompt disabled",
			config:         disablePromptConfig(),
			promptDisabled: true,
		},
		{
			name:           "prompt disabled via GH_PROMPT_DISABLED env var",
			env:            map[string]string{"GH_PROMPT_DISABLED": "1"},
			promptDisabled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					t.Setenv(k, v)
				}
			}
			var cfg gh.Config
			if tt.config != nil {
				cfg = tt.config
			} else {
				cfg = config.NewMockConfig()
			}
			io := newIOStreams(cfg, "")
			assert.Equal(t, tt.promptDisabled, io.GetNeverPrompt())
		})
	}
}

func Test_newIOStreams_spinnerDisabled(t *testing.T) {
	tests := []struct {
		name            string
		config          gh.Config
		invokingAgent   agents.AgentName
		spinnerDisabled bool
		env             map[string]string
	}{
		{
			name:            "default config",
			spinnerDisabled: false,
		},
		{
			name:            "agent detected",
			invokingAgent:   "some-agent",
			spinnerDisabled: true,
		},
		{
			name:            "config with spinner disabled",
			config:          disableSpinnersConfig(),
			spinnerDisabled: true,
		},
		{
			name:            "config with spinner enabled",
			config:          enableSpinnersConfig(),
			spinnerDisabled: false,
		},
		{
			name:            "agent overrides config enabled",
			config:          enableSpinnersConfig(),
			invokingAgent:   "some-agent",
			spinnerDisabled: true,
		},
		{
			name:            "config disabled with agent",
			config:          disableSpinnersConfig(),
			invokingAgent:   "some-agent",
			spinnerDisabled: true,
		},
		{
			name:            "spinner disabled via GH_SPINNER_DISABLED env var = 0",
			env:             map[string]string{"GH_SPINNER_DISABLED": "0"},
			spinnerDisabled: false,
		},
		{
			name:            "spinner disabled via GH_SPINNER_DISABLED env var = false",
			env:             map[string]string{"GH_SPINNER_DISABLED": "false"},
			spinnerDisabled: false,
		},
		{
			name:            "GH_SPINNER_DISABLED false overrides agent",
			invokingAgent:   "some-agent",
			env:             map[string]string{"GH_SPINNER_DISABLED": "false"},
			spinnerDisabled: false,
		},
		{
			name:            "spinner disabled via GH_SPINNER_DISABLED env var = no",
			env:             map[string]string{"GH_SPINNER_DISABLED": "no"},
			spinnerDisabled: false,
		},
		{
			name:            "spinner enabled via GH_SPINNER_DISABLED env var = 1",
			env:             map[string]string{"GH_SPINNER_DISABLED": "1"},
			spinnerDisabled: true,
		},
		{
			name:            "spinner enabled via GH_SPINNER_DISABLED env var = true",
			env:             map[string]string{"GH_SPINNER_DISABLED": "true"},
			spinnerDisabled: true,
		},
		{
			name:            "config enabled but env disabled, respects env",
			config:          enableSpinnersConfig(),
			env:             map[string]string{"GH_SPINNER_DISABLED": "true"},
			spinnerDisabled: true,
		},
		{
			name:            "config disabled but env enabled, respects env",
			config:          disableSpinnersConfig(),
			env:             map[string]string{"GH_SPINNER_DISABLED": "false"},
			spinnerDisabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv registers the cleanup that restores the caller's environment;
			// os.Unsetenv then clears the variable outright. Both are needed because
			// newIOStreams branches on os.LookupEnv, so leaving GH_SPINNER_DISABLED
			// set-but-empty would take the env branch and never reach agent or config.
			t.Setenv("GH_SPINNER_DISABLED", "")
			require.NoError(t, os.Unsetenv("GH_SPINNER_DISABLED"))
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			var cfg gh.Config
			if tt.config != nil {
				cfg = tt.config
			} else {
				cfg = config.NewMockConfig()
			}
			io := newIOStreams(cfg, tt.invokingAgent)
			assert.Equal(t, tt.spinnerDisabled, io.GetSpinnerDisabled())
		})
	}
}

func Test_newIOStreams_accessiblePrompterEnabled(t *testing.T) {
	tests := []struct {
		name                      string
		config                    gh.Config
		accessiblePrompterEnabled bool
		env                       map[string]string
	}{
		{
			name:                      "default config",
			accessiblePrompterEnabled: false,
		},
		{
			name:                      "config with accessible prompter enabled",
			config:                    enableAccessiblePrompterConfig(),
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "config with accessible prompter disabled",
			config:                    disableAccessiblePrompterConfig(),
			accessiblePrompterEnabled: false,
		},
		{
			name:                      "accessible prompter enabled via GH_ACCESSIBLE_PROMPTER env var = 1",
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "1"},
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "accessible prompter enabled via GH_ACCESSIBLE_PROMPTER env var = true",
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "true"},
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "accessible prompter disabled via GH_ACCESSIBLE_PROMPTER env var = 0",
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "0"},
			accessiblePrompterEnabled: false,
		},
		{
			name:                      "config disabled but env enabled, respects env",
			config:                    disableAccessiblePrompterConfig(),
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "true"},
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "config enabled but env disabled, respects env",
			config:                    enableAccessiblePrompterConfig(),
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "false"},
			accessiblePrompterEnabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			var cfg gh.Config
			if tt.config != nil {
				cfg = tt.config
			} else {
				cfg = config.NewMockConfig()
			}
			io := newIOStreams(cfg, "")
			assert.Equal(t, tt.accessiblePrompterEnabled, io.AccessiblePrompterEnabled())
		})
	}
}

func Test_newIOStreams_colorLabels(t *testing.T) {
	tests := []struct {
		name               string
		config             gh.Config
		colorLabelsEnabled bool
		env                map[string]string
	}{
		{
			name:               "default config",
			colorLabelsEnabled: false,
		},
		{
			name:               "config with colorLabels enabled",
			config:             enableColorLabelsConfig(),
			colorLabelsEnabled: true,
		},
		{
			name:               "config with colorLabels disabled",
			config:             disableColorLabelsConfig(),
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels enabled via `1` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "1"},
			colorLabelsEnabled: true,
		},
		{
			name:               "colorLabels enabled via `true` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "true"},
			colorLabelsEnabled: true,
		},
		{
			name:               "colorLabels enabled via `yes` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "yes"},
			colorLabelsEnabled: true,
		},
		{
			name:               "colorLabels disable via empty string in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": ""},
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels disabled via `0` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "0"},
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels disabled via `false` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "false"},
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels disabled via `no` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "no"},
			colorLabelsEnabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					t.Setenv(k, v)
				}
			}
			var cfg gh.Config
			if tt.config != nil {
				cfg = tt.config
			} else {
				cfg = config.NewMockConfig()
			}
			io := newIOStreams(cfg, "")
			assert.Equal(t, tt.colorLabelsEnabled, io.ColorLabels())
		})
	}
}

func Test_mightBeGHESUser(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		cfgString string
		want      bool
	}{
		{
			name: "GH_ENTERPRISE_TOKEN set",
			env:  map[string]string{"GH_ENTERPRISE_TOKEN": "some-token"},
			want: true,
		},
		{
			name: "GITHUB_ENTERPRISE_TOKEN set",
			env:  map[string]string{"GITHUB_ENTERPRISE_TOKEN": "some-token"},
			want: true,
		},
		{
			name:      "no env vars, config has enterprise host",
			cfgString: "hosts:\n  ghes.example.com:\n    oauth_token: abc123\n",
			want:      true,
		},
		{
			name:      "no env vars, config has only github.com",
			cfgString: "hosts:\n  github.com:\n    oauth_token: abc123\n",
			want:      false,
		},
		{
			name: "no env vars, config has no hosts",
			want: false,
		},
		{
			name:      "no env vars, config has github.com and enterprise host",
			cfgString: "hosts:\n  github.com:\n    oauth_token: abc123\n  ghes.example.com:\n    oauth_token: def456\n",
			want:      true,
		},
		{
			name:      "no env vars, config has tenancy host",
			cfgString: "hosts:\n  my-company.ghe.com:\n    oauth_token: abc123\n",
			want:      false,
		},
		{
			name: "GH_HOST set to enterprise host",
			env:  map[string]string{"GH_HOST": "ghes.example.com"},
			want: true,
		},
		{
			name: "GH_HOST set to github.com",
			env:  map[string]string{"GH_HOST": "github.com"},
			want: false,
		},
		{
			name: "GH_HOST set to tenancy host",
			env:  map[string]string{"GH_HOST": "my-company.ghe.com"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := config.NewIsolatedTestConfig(t, tt.cfgString)

			// Set after isolating the config, which clears the auth env vars.
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := mightBeGHESUser(cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func pagerConfig() gh.Config {
	return config.NewMockConfigFromString("pager: CONFIG_PAGER")
}

func disablePromptConfig() gh.Config {
	return config.NewMockConfigFromString("prompt: disabled")
}

func enableAccessiblePrompterConfig() gh.Config {
	return config.NewMockConfigFromString("accessible_prompter: enabled")
}

func disableAccessiblePrompterConfig() gh.Config {
	return config.NewMockConfigFromString("accessible_prompter: disabled")
}

func disableSpinnersConfig() gh.Config {
	return config.NewMockConfigFromString("spinner: disabled")
}

func enableSpinnersConfig() gh.Config {
	return config.NewMockConfigFromString("spinner: enabled")
}

func disableColorLabelsConfig() gh.Config {
	return config.NewMockConfigFromString("color_labels: disabled")
}

func enableColorLabelsConfig() gh.Config {
	return config.NewMockConfigFromString("color_labels: enabled")
}

func Test_authRecoveryCommand(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		source     string
		requestURL string
		want       string
	}{
		{
			name:       "stored oauth token",
			token:      "gho_abc123",
			source:     "oauth_token",
			requestURL: "https://api.github.com/graphql",
			want:       "gh auth refresh -h github.com",
		},
		{
			name:       "stored pat",
			token:      "github_pat_abc123",
			source:     "oauth_token",
			requestURL: "https://api.github.com/graphql",
			want:       "gh auth login -h github.com",
		},
		{
			name:       "env token",
			token:      "gho_abc123",
			source:     "GH_TOKEN",
			requestURL: "https://api.github.com/graphql",
			want:       "gh auth login -h github.com",
		},
		{
			name:   "missing request url",
			token:  "gho_abc123",
			source: "oauth_token",
			want:   "gh auth login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authCfg := config.NewMockConfig().Authentication()
			authCfg.SetActiveToken(tt.token, tt.source)
			cfg := &ghmock.ConfigMock{
				AuthenticationFunc: func() gh.AuthConfig {
					return authCfg
				},
			}

			var requestURL *url.URL
			if tt.requestURL != "" {
				var err error
				requestURL, err = url.Parse(tt.requestURL)
				if err != nil {
					t.Fatalf("failed to parse request URL: %v", err)
				}
			}

			httpErr := api.HTTPError{
				HTTPError: &ghAPI.HTTPError{
					RequestURL: requestURL,
					StatusCode: 401,
				},
			}

			got := authRecoveryCommand(cfg, httpErr)
			if got != tt.want {
				t.Errorf("authRecoveryCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}
