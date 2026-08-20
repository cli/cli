package ghcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/internal/gherrs"
	"github.com/cli/cli/v2/internal/telemetry"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			io := newIOStreams(cfg)
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
			io := newIOStreams(cfg)
			assert.Equal(t, tt.promptDisabled, io.GetNeverPrompt())
		})
	}
}

func Test_newIOStreams_spinnerDisabled(t *testing.T) {
	tests := []struct {
		name            string
		config          gh.Config
		spinnerDisabled bool
		env             map[string]string
	}{
		{
			name:            "default config",
			spinnerDisabled: false,
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
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			var cfg gh.Config
			if tt.config != nil {
				cfg = tt.config
			} else {
				cfg = config.NewMockConfig()
			}
			io := newIOStreams(cfg)
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
			io := newIOStreams(cfg)
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
			io := newIOStreams(cfg)
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

func Test_grabAllUnwrappableNestedErrorTypes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "generic error is noise and filtered",
			err:  errors.New("boom"),
			want: "",
		},
		{
			name: "single fmt wrap filters noise",
			err:  fmt.Errorf("context: %w", gherrs.SilentError),
			want: "gherrs.silentError",
		},
		{
			name: "double fmt wrap filters noise",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", gherrs.SilentError)),
			want: "gherrs.silentError",
		},
		{
			name: "errors.Join captures interesting branches",
			err:  errors.Join(errors.New("a"), fmt.Errorf("wrap: %w", gherrs.SilentError)),
			want: "gherrs.silentError",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grabAllUnwrappableNestedErrorTypes(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_grabAllUnwrappableNestedErrorTypes_boundedByDepthLimit(t *testing.T) {
	// Build a chain deeper than the 100-element bound using a non-noise
	// error type so that entries are not filtered out.
	err := gherrs.GeneralError{Message: "leaf"}
	var wrapped error = err
	for range 101 {
		wrapped = gherrs.GeneralError{WrappedErr: wrapped, Message: "wrap"}
	}
	got := grabAllUnwrappableNestedErrorTypes(wrapped)
	parts := strings.Split(got, ",")
	assert.Len(t, parts, 100, "should be bounded to 100 entries")
}

func Test_newErrDims(t *testing.T) {
	tests := []struct {
		name        string
		mappedErr   error
		originalErr error
		want        ghtelemetry.Dimensions
	}{
		{
			name:        "generic error is failure",
			mappedErr:   errors.New("boom"),
			originalErr: errors.New("boom"),
			want:        ghtelemetry.Dimensions{"outcome": "error", "errTypes": ""},
		},
		{
			name:        "silent error is failure",
			mappedErr:   gherrs.SilentError,
			originalErr: gherrs.SilentError,
			want:        ghtelemetry.Dimensions{"outcome": "error", "errTypes": "gherrs.silentError"},
		},
		{
			name:        "user cancellation error is success",
			mappedErr:   gherrs.UserCancellationError,
			originalErr: gherrs.UserCancellationError,
			want:        ghtelemetry.Dimensions{"outcome": "success", "errTypes": "gherrs.userCancellationError"},
		},
		{
			name:        "wrapped user cancellation error is success",
			mappedErr:   gherrs.UserCancellationError,
			originalErr: fmt.Errorf("wrapped: %w", gherrs.UserCancellationError),
			want:        ghtelemetry.Dimensions{"outcome": "success", "errTypes": "gherrs.userCancellationError"},
		},
		{
			name:        "pending error is success",
			mappedErr:   gherrs.PendingError,
			originalErr: gherrs.PendingError,
			want:        ghtelemetry.Dimensions{"outcome": "success", "errTypes": "gherrs.pendingError"},
		},
		{
			name:        "mapped sentinel preserves original error chain for errTypes",
			mappedErr:   gherrs.SilentError,
			originalErr: fmt.Errorf("context: %w", errors.New("root cause")),
			want:        ghtelemetry.Dimensions{"outcome": "error", "errTypes": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, newErrDims(tt.mappedErr, tt.originalErr))
		})
	}
}

// runMainAndCaptureTelemetry runs Main() with the provided args in an isolated
// config/state directory with telemetry logging enabled, and returns the exit
// code and the telemetry payloads that were flushed to stderr.
//
// This is intentionally a janky end-to-end test: Main() reads from os.Args and
// writes to os.Stderr/os.Stdout, so we swap those process-globals around the
// call. Tests using this helper cannot run in parallel.
//
// The expectation in the long run is that we will pull this out of ghcmd.Main.
func runMainAndCaptureTelemetry(t *testing.T, args []string) (exitCode, []telemetry.SendTelemetryPayload) {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tmpDir)
	t.Setenv("GH_DATA_DIR", tmpDir)
	t.Setenv("GH_STATE_DIR", tmpDir)

	// Enable telemetry in log mode so payloads are written to stderr rather than
	// spawned as a subprocess.
	t.Setenv("GH_PRIVATE_ENABLE_TELEMETRY", "1")
	t.Setenv("GH_TELEMETRY", "log")

	// Prevent being classified as a GHES user, which would force NoOpService.
	t.Setenv("GH_HOST", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")

	// Clear GitHub.com auth tokens so tests don't accidentally make real API
	// requests when running in environments that have them set.
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// Disable color to keep the LogFlusher output easy to parse.
	t.Setenv("NO_COLOR", "1")

	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = args

	origStderr := os.Stderr
	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = stderrW
	t.Cleanup(func() { os.Stderr = origStderr })

	origStdout := os.Stdout
	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = stdoutW
	t.Cleanup(func() { os.Stdout = origStdout })

	// Drain stdout and stderr in the background so command output does not
	// fill the pipe buffer and block the process.
	stdoutDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdoutR)
		close(stdoutDone)
	}()

	var stderrBytes []byte
	stderrDone := make(chan struct{})
	go func() {
		stderrBytes, _ = io.ReadAll(stderrR)
		close(stderrDone)
	}()

	exit := Main()

	require.NoError(t, stderrW.Close())
	require.NoError(t, stdoutW.Close())
	<-stdoutDone
	<-stderrDone

	return exit, parseTelemetryPayloads(t, stderrBytes)
}

// parseTelemetryPayloads extracts JSON payloads written by telemetry.LogFlusher
// from the given stderr capture. LogFlusher prefixes each payload with a
// "Telemetry payload:" header followed by indented JSON.
func parseTelemetryPayloads(t *testing.T, stderr []byte) []telemetry.SendTelemetryPayload {
	t.Helper()

	const header = "Telemetry payload:"
	var payloads []telemetry.SendTelemetryPayload

	remaining := stderr
	for {
		idx := bytes.Index(remaining, []byte(header))
		if idx < 0 {
			break
		}
		remaining = remaining[idx+len(header):]

		// Locate the JSON object starting with '{' and let json.Decoder parse
		// exactly one payload so braces inside JSON strings do not affect
		// boundary detection.
		start := bytes.IndexByte(remaining, '{')
		if start < 0 {
			break
		}

		decoder := json.NewDecoder(bytes.NewReader(remaining[start:]))
		var p telemetry.SendTelemetryPayload
		require.NoError(t, decoder.Decode(&p))
		payloads = append(payloads, p)

		consumed := int(decoder.InputOffset())
		if consumed <= 0 {
			break
		}
		remaining = remaining[start+consumed:]
	}

	return payloads
}

func findCommandInvocation(payloads []telemetry.SendTelemetryPayload) *telemetry.PayloadEvent {
	for _, p := range payloads {
		for i := range p.Events {
			if p.Events[i].Type == "command_invocation" {
				return &p.Events[i]
			}
		}
	}
	return nil
}

func TestMain_recordsCommandInvocationTelemetry_versionSubcommand(t *testing.T) {
	exit, payloads := runMainAndCaptureTelemetry(t, []string{"gh", "version"})

	assert.Equal(t, exitOK, exit)

	event := findCommandInvocation(payloads)
	require.NotNil(t, event, "expected a command_invocation event in telemetry payloads")

	assert.Equal(t, "gh version", event.Dimensions["command"])
	// Successful runs do not populate outcome/errTypes, only error runs do.
	assert.NotContains(t, event.Dimensions, "outcome")
	assert.NotContains(t, event.Dimensions, "errTypes")
}

func TestMain_recordsCommandInvocationTelemetry_flagError(t *testing.T) {
	exit, payloads := runMainAndCaptureTelemetry(t, []string{"gh", "version", "--definitely-not-a-real-flag"})

	assert.Equal(t, exitError, exit)

	event := findCommandInvocation(payloads)
	require.NotNil(t, event, "expected a command_invocation event in telemetry payloads")

	assert.Equal(t, "gh version", event.Dimensions["command"])
	assert.Equal(t, "error", event.Dimensions["outcome"])
	assert.NotEmpty(t, event.Dimensions["errTypes"], "errTypes should be populated on error")
}

func TestMain_recordsCommandInvocationTelemetry_flagsDimension(t *testing.T) {
	exit, payloads := runMainAndCaptureTelemetry(t, []string{"gh", "api", "--method", "GET", "--hostname", "example.com", "/"})

	// The command will fail (no auth), but that's fine - we only care about
	// the flags dimension being present and sorted.
	_ = exit

	event := findCommandInvocation(payloads)
	require.NotNil(t, event, "expected a command_invocation event in telemetry payloads")

	assert.Equal(t, "gh api", event.Dimensions["command"])
	flags := event.Dimensions["flags"]
	assert.Contains(t, flags, "hostname")
	assert.Contains(t, flags, "method")
	// Flags should be sorted alphabetically.
	assert.Equal(t, "hostname,method", flags)
}

func TestMain_telemetryDisabledCommand_noCommandInvocationEvent(t *testing.T) {
	exit, payloads := runMainAndCaptureTelemetry(t, []string{"gh", "completion"})

	assert.Equal(t, exitOK, exit)

	event := findCommandInvocation(payloads)
	assert.Nil(t, event, "telemetry-disabled commands should not emit command_invocation")
}
