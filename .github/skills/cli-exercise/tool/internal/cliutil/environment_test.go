package cliutil

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsolatedEnvironment(t *testing.T) {
	t.Setenv("GH_TOKEN", "synthetic-must-not-inherit")
	t.Setenv("XDG_CONFIG_HOME", "/not-the-fixture")
	for _, prefix := range []string{"", "."} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			root := t.TempDir()
			environment, err := IsolatedEnvironment(root, prefix)
			require.NoError(t, err)
			require.Equal(t, filepath.Join(root, prefix+"home"), environment["HOME"])
			require.Equal(t, environment["HOME"], environment["USERPROFILE"])
			require.Equal(t, filepath.Join(root, prefix+"config"), environment["XDG_CONFIG_HOME"])
			require.NotContains(t, environment, "GH_TOKEN")
			require.NotContains(t, environment, "PATH", "each caller chooses its search path explicitly")
			for _, directory := range environment {
				require.DirExists(t, directory)
			}
			require.True(t, slices.IsSorted(EnvironmentList(environment)))
			again, err := IsolatedEnvironment(root, prefix)
			require.NoError(t, err)
			require.Equal(t, environment, again)
		})
	}
	t.Run("reject symbolic profile", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "home")))
		_, err := IsolatedEnvironment(root, "")
		require.ErrorContains(t, err, "symbolic links")
	})
}
