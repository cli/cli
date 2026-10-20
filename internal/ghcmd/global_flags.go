package ghcmd

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/internal/globalflags"
)

type globalFlagStartupOptions struct {
	serverURL          string
	cacheDir           string
	telemetrySender    bool
	externalHTTPClient func() (*http.Client, error)
}

func loadGlobalFlagSnapshot(ctx context.Context, opts globalFlagStartupOptions) (globalflags.Snapshot, error) {
	if opts.serverURL == "" || opts.telemetrySender {
		return globalflags.Snapshot{}, nil
	}
	client, err := opts.externalHTTPClient()
	if err != nil {
		return globalflags.Snapshot{}, err
	}
	return globalflags.Load(ctx, opts.serverURL, opts.cacheDir, client)
}
