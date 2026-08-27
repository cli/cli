package browser

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type browseFunc func(string) error

func (f browseFunc) Browse(url string) error {
	return f(url)
}

func TestFallbackBrowser_Browse(t *testing.T) {
	innerErr := errors.New("open url failed")
	opened := [][]string{}
	appRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(appRoot, "Microsoft Edge.app"), 0o755))

	b := &fallbackBrowser{
		inner: browseFunc(func(string) error {
			return innerErr
		}),
		exec: func(name string, arg ...string) error {
			opened = append(opened, append([]string{name}, arg...))
			return nil
		},
		stat: os.Stat,
		apps: []string{"Microsoft Edge", "Safari"},
		dirs: []string{appRoot},
	}

	err := b.Browse("https://github.com/login/device")
	require.NoError(t, err)
	assert.Equal(t, [][]string{
		{"open", "-a", "Microsoft Edge", "https://github.com/login/device"},
	}, opened)
}

func TestFallbackBrowser_BrowseKeepsOriginalError(t *testing.T) {
	innerErr := errors.New("open url failed")
	b := &fallbackBrowser{
		inner: browseFunc(func(string) error {
			return innerErr
		}),
		exec: func(string, ...string) error {
			t.Fatal("should not launch a fallback browser when none are installed")
			return nil
		},
		stat: os.Stat,
		apps: []string{"Microsoft Edge", "Safari"},
		dirs: []string{t.TempDir()},
	}

	err := b.Browse("https://github.com/login/device")
	require.Equal(t, innerErr, err)
}

func TestFallbackBrowser_BrowseSkipsFallbackOnSuccess(t *testing.T) {
	b := &fallbackBrowser{
		inner: browseFunc(func(string) error {
			return nil
		}),
		exec: func(string, ...string) error {
			t.Fatal("should not launch a fallback browser when the default opener succeeds")
			return nil
		},
		stat: func(string) (os.FileInfo, error) {
			t.Fatal("should not look for fallback browsers when the default opener succeeds")
			return nil, os.ErrNotExist
		},
	}

	require.NoError(t, b.Browse("https://github.com/login/device"))
}

func TestOpenFirstInstalledApp(t *testing.T) {
	appRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(appRoot, "Safari.app"), 0o755))

	var opened [][]string
	err := openFirstInstalledApp(
		"https://example.com",
		[]string{"Microsoft Edge", "Safari"},
		[]string{appRoot},
		func(name string, arg ...string) error {
			opened = append(opened, append([]string{name}, arg...))
			return nil
		},
		os.Stat,
	)

	require.NoError(t, err)
	assert.Equal(t, [][]string{
		{"open", "-a", "Safari", "https://example.com"},
	}, opened)
}

func TestOpenFirstInstalledAppTriesNextApp(t *testing.T) {
	appRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(appRoot, "Microsoft Edge.app"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(appRoot, "Safari.app"), 0o755))

	var opened [][]string
	err := openFirstInstalledApp(
		"https://example.com",
		[]string{"Microsoft Edge", "Safari"},
		[]string{appRoot},
		func(name string, arg ...string) error {
			opened = append(opened, append([]string{name}, arg...))
			if arg[1] == "Microsoft Edge" {
				return errors.New("failed to launch Edge")
			}
			return nil
		},
		os.Stat,
	)

	require.NoError(t, err)
	assert.Equal(t, [][]string{
		{"open", "-a", "Microsoft Edge", "https://example.com"},
		{"open", "-a", "Safari", "https://example.com"},
	}, opened)
}

func TestHasExplicitLauncher(t *testing.T) {
	t.Run("launcher argument", func(t *testing.T) {
		assert.True(t, hasExplicitLauncher("open -a Safari"))
	})

	t.Run("GH_BROWSER", func(t *testing.T) {
		t.Setenv("GH_BROWSER", "false")
		t.Setenv("BROWSER", "")
		assert.True(t, hasExplicitLauncher(""))
	})

	t.Run("BROWSER", func(t *testing.T) {
		t.Setenv("GH_BROWSER", "")
		t.Setenv("BROWSER", "false")
		assert.True(t, hasExplicitLauncher(""))
	})
}

func TestNewSkipsFallbackWhenBrowserEnvSet(t *testing.T) {
	t.Setenv("BROWSER", "false")
	t.Setenv("GH_BROWSER", "")

	b := New("", io.Discard, io.Discard)
	_, isFallback := b.(*fallbackBrowser)
	assert.False(t, isFallback, "BROWSER=false must keep the documented no-browser workaround")
}
