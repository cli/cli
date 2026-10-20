package refreshglobalfeatureflags

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/cli/cli/v2/internal/globalflags"
	"github.com/cli/cli/v2/pkg/cmdutil"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
	"github.com/spf13/cobra"
)

type refreshFunc func(context.Context, string, string, *http.Client) error

// NewCmdRefreshGlobalFeatureFlags creates the hidden global flag cache worker.
func NewCmdRefreshGlobalFeatureFlags(f *cmdutil.Factory) *cobra.Command {
	return newCmdRefreshGlobalFeatureFlags(f, ghConfig.CacheDir(), globalflags.Refresh)
}

func newCmdRefreshGlobalFeatureFlags(f *cmdutil.Factory, cacheDir string, refresh refreshFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "refresh-global-feature-flags",
		Short:  "Refresh global feature flags",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			serverURL := os.Getenv("GH_GLOBAL_FEATURE_FLAG_SERVER_URL")
			if serverURL == "" {
				return errors.New("GH_GLOBAL_FEATURE_FLAG_SERVER_URL is not set")
			}
			client, err := f.ExternalHttpClient()
			if err != nil {
				return err
			}
			return refresh(cmd.Context(), serverURL, cacheDir, client)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.DisableTelemetry(cmd)
	return cmd
}
