package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScriptReleaseDispatchesPrerelease(t *testing.T) {
	root := repositoryRoot(t)
	stubDir := t.TempDir()
	record := filepath.Join(stubDir, "args")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"${GH_RELEASE_STUB_ARGS:?}\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(stubDir, "gh"), []byte(stub), 0o755))

	cmd := exec.Command(filepath.Join(root, "script", "release"), "v9.9.9-release-probe")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_RELEASE_STUB_ARGS="+record,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "script/release:\n%s", out)

	recorded, err := os.ReadFile(record)
	require.NoError(t, err)
	args := strings.Split(strings.TrimRight(string(recorded), "\n"), "\n")
	joined := strings.Join(args, " ")
	require.NotContains(t, joined, "cli/cli")
	require.NotContains(t, joined, "deployment.yml")
	require.Equal(t, []string{
		"workflow",
		"-R",
		originSlug(t, root),
		"run",
		"prerelease.yml",
		"--ref",
		"trunk",
	}, args)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(filepath.Dir(file))
}

func originSlug(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", root, "remote", "get-url", "origin")
	out, err := cmd.Output()
	require.NoError(t, err)
	return gitRemoteSlug(t, strings.TrimSpace(string(out)))
}

func gitRemoteSlug(t *testing.T, url string) string {
	t.Helper()
	url = strings.TrimSuffix(url, ".git")
	var path string
	switch {
	case strings.HasPrefix(url, "git@") && strings.Contains(url, ":"):
		_, path, _ = strings.Cut(url, ":")
	case strings.Contains(url, "://"):
		rest := url[strings.Index(url, "://")+3:]
		if i := strings.Index(rest, "@"); i >= 0 {
			rest = rest[i+1:]
		}
		if i := strings.Index(rest, "/"); i >= 0 {
			path = rest[i+1:]
		}
	default:
		t.Fatalf("unrecognized origin url %q", url)
	}
	require.Contains(t, path, "/")
	return path
}
