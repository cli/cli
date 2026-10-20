package ghcmd

import (
	"errors"
	"os/exec"

	"github.com/cli/cli/v2/internal/detach"
	"github.com/cli/cli/v2/internal/globalflags"
)

func startGlobalFlagRefresh(executable string) error {
	cmd := exec.Command(executable, "refresh-global-feature-flags")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = detach.SysProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
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
	return snapshot, errors.Join(cacheErr, opts.startRefresh())
}
