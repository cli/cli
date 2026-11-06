package ghcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/detach"
	"github.com/cli/cli/v2/internal/globalflags"
)

const globalFlagRefreshCommand = "refresh-global-feature-flags"

func startGlobalFlagRefresh(executable string) error {
	if abs, err := filepath.Abs(executable); err == nil {
		executable = abs
	}
	cmd := exec.Command(executable, globalFlagRefreshCommand)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if filepath.IsAbs(executable) {
		cmd.Dir = os.TempDir()
	}
	cmd.SysProcAttr = detach.SysProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func isGlobalFlagRefreshWorker(args []string) bool {
	return len(args) == 2 && args[1] == globalFlagRefreshCommand
}

func runGlobalFlagRefreshWorker(ctx context.Context, serverURL, cacheDir, appVersion string, stderr io.Writer) error {
	if serverURL == "" {
		return errors.New("GH_GLOBAL_FEATURE_FLAG_SERVER_URL is not set")
	}
	client, err := api.NewExternalHTTPClient(api.ExternalHTTPClientOptions{
		AppVersion: appVersion,
		Log:        stderr,
	})
	if err != nil {
		return err
	}
	if err := globalflags.Refresh(ctx, serverURL, cacheDir, client); err != nil {
		return errors.Join(err, globalflags.RecordRefreshFailure(serverURL, cacheDir, err))
	}
	return nil
}

type globalFlagStartupOptions struct {
	serverURL    string
	cacheDir     string
	skipRefresh  bool
	startRefresh func() error
}

func loadGlobalFlagSnapshot(opts globalFlagStartupOptions) (globalflags.Snapshot, error) {
	if opts.serverURL == "" || opts.skipRefresh {
		return globalflags.Snapshot{}, nil
	}
	snapshot, fresh, cacheErr := globalflags.FromCache(opts.serverURL, opts.cacheDir)
	if fresh {
		return snapshot, cacheErr
	}
	request, requestErr := globalflags.BeginRefresh(opts.serverURL, opts.cacheDir)
	if request == nil {
		return snapshot, errors.Join(cacheErr, requestErr)
	}
	startErr := opts.startRefresh()
	var cancelErr error
	if startErr != nil {
		cancelErr = request.Cancel()
	}
	return snapshot, errors.Join(cacheErr, requestErr, startErr, cancelErr)
}
