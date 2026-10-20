package refreshglobalfeatureflags

import (
	"context"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshGlobalFeatureFlagsWarmsCacheWithoutAuthOrTelemetry(t *testing.T) {
	// Given the hidden refresh worker's development endpoint and dependencies
	t.Setenv("GH_GLOBAL_FEATURE_FLAG_SERVER_URL", "https://flags.example")
	var gotServerURL, gotCacheDir string
	var gotClient *http.Client
	client := &http.Client{}
	f := &cmdutil.Factory{
		ExternalHttpClient: func() (*http.Client, error) {
			return client, nil
		},
	}

	// When the worker command executes
	cmd := newCmdRefreshGlobalFeatureFlags(f, "/cache", func(ctx context.Context, serverURL, cacheDir string, httpClient *http.Client) error {
		gotServerURL = serverURL
		gotCacheDir = cacheDir
		gotClient = httpClient
		return nil
	})
	root := &cobra.Command{Use: "gh"}
	root.AddCommand(cmd)
	root.SetArgs([]string{"refresh-global-feature-flags"})
	err := root.Execute()

	// Then it refreshes the global cache without auth or telemetry instrumentation
	require.NoError(t, err)
	assert.Equal(t, "https://flags.example", gotServerURL)
	assert.Equal(t, "/cache", gotCacheDir)
	assert.Same(t, client, gotClient)
	assert.False(t, cmdutil.IsAuthCheckEnabled(cmd), "the worker must run without GitHub authentication")
	assert.Equal(t, "disabled", cmd.Annotations["telemetry"])
	assert.True(t, cmd.Hidden, "the worker is infrastructure, not a user-facing command")
}
