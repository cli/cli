package ghcmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/pkg/cmd/root"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMainLoadsGlobalFlagsBeforeConstructingCommands(t *testing.T) {
	t.Parallel()

	// Given a global service, regardless of configured GitHub tokens, host, or repository
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "startup must use the unauthenticated client")
		_, _ = io.WriteString(w, `{"flags":{"constructor-test":true}}`)
	}))
	defer server.Close()

	// When the real CLI startup constructs its complete command tree for help
	stdout, stderr := runGlobalFlagMain(t, []string{"--help"}, []string{
		"GH_GLOBAL_FEATURE_FLAG_SERVER_URL=" + server.URL,
		"GH_TEST_EXPECT_GLOBAL_FLAG=1",
	})

	// Then the constructor probe saw the snapshot before eager root construction
	assert.Contains(t, stdout, "USAGE")
	assert.Empty(t, stderr)
}

func TestMainSkipsGlobalLookupForTelemetrySubprocess(t *testing.T) {
	t.Parallel()

	// Given an opted-in endpoint that an internal telemetry sender must not query
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("telemetry subprocess unexpectedly requested global flags")
		_, _ = io.WriteString(w, `{"flags":{"constructor-test":true}}`)
	}))
	defer server.Close()

	// When the existing sender handles an empty event batch
	stdout, stderr := runGlobalFlagMain(t, []string{"send-telemetry"}, []string{
		"GH_GLOBAL_FEATURE_FLAG_SERVER_URL=" + server.URL,
	})

	// Then it retains default flags and does no extra work or output
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestMainReportsGlobalFlagFailuresOnlyInDebugMode(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		debug       string
		wantWarning bool
	}{
		{debug: "", wantWarning: false},
		{debug: "1", wantWarning: true},
	} {
		t.Run("GH_DEBUG="+tt.debug, func(t *testing.T) {
			t.Parallel()

			// Given an unavailable global service
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"code":"unavailable","msg":"try later"}`)
			}))
			defer server.Close()

			// When startup falls back to defaults
			stdout, stderr := runGlobalFlagMain(t, []string{"--help"}, []string{
				"GH_GLOBAL_FEATURE_FLAG_SERVER_URL=" + server.URL,
				"GH_DEBUG=" + tt.debug,
			})

			// Then commands remain usable and diagnostics follow the existing debug policy
			assert.Contains(t, stdout, "USAGE")
			assert.Equal(t, tt.wantWarning, strings.Contains(stderr,
				"warning: loading global feature flags failed: global flags: HTTP 503"),
				"lookup failures should be observable only when debugging is enabled")
		})
	}
}

// This subprocess seam observes the Factory at the real root-construction
// boundary without changing any production command's visibility.
func TestMainGlobalFlagsProcess(t *testing.T) {
	if os.Getenv("GH_TEST_GLOBAL_FLAGS_PROCESS") != "1" {
		t.Skip("subprocess helper")
	}
	newCmdRoot = func(f *cmdutil.Factory, recorder ghtelemetry.InvocationRecorder, version, date string) (*cobra.Command, error) {
		want := os.Getenv("GH_TEST_EXPECT_GLOBAL_FLAG") == "1"
		if got := f.GlobalFlags.Enabled("constructor-test"); got != want {
			return nil, fmt.Errorf("global snapshot at root construction: got %t, want %t", got, want)
		}
		return root.NewCmdRoot(f, recorder, version, date)
	}
	os.Args = append([]string{"gh"}, os.Args[3:]...)
	os.Exit(int(Main()))
}

func runGlobalFlagMain(t *testing.T, args, env []string) (string, string) {
	t.Helper()
	home := t.TempDir()
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(executable, append([]string{"-test.run=^TestMainGlobalFlagsProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(),
		"GH_TEST_GLOBAL_FLAGS_PROCESS=1",
		"GH_TEST_EXPECT_GLOBAL_FLAG=",
		"HOME="+home,
		"USERPROFILE="+home,
		"GH_CONFIG_DIR="+filepath.Join(home, "config"),
		"XDG_DATA_HOME="+filepath.Join(home, "data"),
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"GH_GLOBAL_FEATURE_FLAG_SERVER_URL=",
		"GH_TELEMETRY=disabled",
		"GH_NO_UPDATE_NOTIFIER=1",
		"GH_PROMPT_DISABLED=1",
		"GH_HOST=tenant.ghe.com",
		"GH_REPO=elsewhere.example/owner/repo",
		"GH_TOKEN=test-token",
		"GITHUB_TOKEN=test-token",
		"GH_ENTERPRISE_TOKEN=test-enterprise-token",
		"GITHUB_ENTERPRISE_TOKEN=test-enterprise-token",
		"GH_DEBUG=", "DEBUG=", "GH_COBRA=", "GH_PATH=",
		"GH_PAGER=", "PAGER=", "GH_FORCE_TTY=", "NO_COLOR=1",
	)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(`{"events":[]}`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), "stdout:\n%s\nstderr:\n%s", &stdout, &stderr)
	return stdout.String(), stderr.String()
}
