//go:build !windows

package ghcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartGlobalFlagRefreshSurvivesInvokerExit(t *testing.T) {
	if os.Getenv("GH_TEST_GLOBAL_FLAG_REFRESH_PARENT") == "1" {
		require.NoError(t, startGlobalFlagRefresh(os.Getenv("GH_TEST_GLOBAL_FLAG_REFRESH_EXECUTABLE")))
		os.Exit(0)
	}

	tempDir := t.TempDir()
	marker := filepath.Join(tempDir, "refreshed")
	worker := filepath.Join(tempDir, "worker")
	require.NoError(t, os.WriteFile(worker, []byte("#!/bin/sh\nsleep 0.2\nprintf refreshed > \"$GH_TEST_GLOBAL_FLAG_REFRESH_MARKER\"\n"), 0o700))
	testExecutable, err := os.Executable()
	require.NoError(t, err)
	parent := exec.Command(testExecutable, "-test.run=^TestStartGlobalFlagRefreshSurvivesInvokerExit$")
	parent.Env = append(os.Environ(),
		"GH_TEST_GLOBAL_FLAG_REFRESH_PARENT=1",
		"GH_TEST_GLOBAL_FLAG_REFRESH_EXECUTABLE="+worker,
		"GH_TEST_GLOBAL_FLAG_REFRESH_MARKER="+marker,
	)

	require.NoError(t, parent.Run(), "the invoking process should exit without waiting for refresh")
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(marker)
		return err == nil && string(data) == "refreshed"
	}, 2*time.Second, 10*time.Millisecond, "the detached refresh should finish after its invoker exits")
}
