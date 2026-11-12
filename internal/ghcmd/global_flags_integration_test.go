package ghcmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/globalflags"
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

	// When an earlier refresh has populated the cache and the real CLI starts
	stdout, stderr := runGlobalFlagMainWithSetup(t, []string{"--help"}, []string{
		"GH_GLOBAL_FEATURE_FLAG_SERVER_URL=" + server.URL,
		"GH_TEST_EXPECT_GLOBAL_FLAG=1",
	}, func(cacheDir string) {
		require.NoError(t, globalflags.Refresh(t.Context(), server.URL, cacheDir, server.Client()))
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

func TestMainReportsRefreshStartFailuresOnlyInDebugMode(t *testing.T) {
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

			// Given an opted-in endpoint whose refresh process cannot start
			stdout, stderr := runGlobalFlagMain(t, []string{"--help"}, []string{
				"GH_GLOBAL_FEATURE_FLAG_SERVER_URL=https://flags.example",
				"GH_PATH=" + filepath.Join(t.TempDir(), "missing-gh"),
				"GH_DEBUG=" + tt.debug,
			})

			// Then commands remain usable and diagnostics follow the existing debug policy
			assert.Contains(t, stdout, "USAGE")
			assert.Equal(t, tt.wantWarning, strings.Contains(stderr,
				"warning: global feature flag startup failed:"),
				"refresh start failures should be observable only when debugging is enabled")
		})
	}
}

func TestMainRunsGlobalFlagWorkerBeforeNormalStartup(t *testing.T) {
	// Given a global endpoint and normal startup that must not run in the worker
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "the worker must remain independent of GitHub credentials")
		_, _ = io.WriteString(w, `{"flags":{"constructor-test":true}}`)
	}))
	defer server.Close()
	home := t.TempDir()
	t.Setenv("GH_GLOBAL_FEATURE_FLAG_SERVER_URL", server.URL)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "config"))
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GH_TELEMETRY", "log")
	originalArgs := os.Args
	originalNewCmdRoot := newCmdRoot
	t.Cleanup(func() {
		os.Args = originalArgs
		newCmdRoot = originalNewCmdRoot
	})
	os.Args = []string{"gh", "refresh-global-feature-flags"}
	rootConstructed := false
	newCmdRoot = func(*cmdutil.Factory, ghtelemetry.InvocationRecorder, string, string) (*cobra.Command, error) {
		rootConstructed = true
		return nil, errors.New("normal root construction must not run")
	}

	// When the internal worker is dispatched through Main
	code := Main()

	// Then it refreshes the cache without entering ordinary CLI startup
	assert.Equal(t, exitOK, code)
	assert.False(t, rootConstructed, "the worker must bypass config, telemetry, updates, and root construction")
	snapshot, fresh, err := globalflags.FromCache(server.URL, filepath.Join(home, "cache", "gh"))
	require.NoError(t, err)
	assert.True(t, fresh, "the worker should persist a fresh snapshot")
	assert.True(t, snapshot.Enabled("constructor-test"), "the worker should persist the server response")
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
	return runGlobalFlagMainWithSetup(t, args, env, nil)
}

func runGlobalFlagMainWithSetup(t *testing.T, args, env []string, setup func(string)) (string, string) {
	t.Helper()
	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache", "gh")
	if setup != nil {
		setup(cacheDir)
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(executable, append([]string{"-test.run=^TestMainGlobalFlagsProcess$", "--"}, args...)...)
	overrides := append([]string{
		"GH_TEST_GLOBAL_FLAGS_PROCESS=1",
		"GH_TEST_EXPECT_GLOBAL_FLAG=",
		"HOME=" + home,
		"USERPROFILE=" + home,
		"GH_CONFIG_DIR=" + filepath.Join(home, "config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
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
	}, env...)
	envMap := map[string]string{}
	for _, entry := range append(os.Environ(), overrides...) {
		key, value, found := strings.Cut(entry, "=")
		if found {
			envMap[key] = value
		}
	}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cmd.Env = append(cmd.Env, key+"="+envMap[key])
	}
	cmd.Stdin = strings.NewReader(`{"events":[]}`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), "stdout:\n%s\nstderr:\n%s", &stdout, &stderr)
	return stdout.String(), stderr.String()
}
