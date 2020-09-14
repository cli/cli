package factory

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
)

func New(appVersion string) *cmdutil.Factory {
	io := iostreams.System()

	var cachedConfig config.Config
	var configError error
	configFunc := func() (config.Config, error) {
		if cachedConfig != nil || configError != nil {
			return cachedConfig, configError
		}
		cachedConfig, configError = config.ParseDefaultConfig()
		if errors.Is(configError, os.ErrNotExist) {
			cachedConfig = config.NewBlankConfig()
			configError = nil
		}
		cachedConfig = config.InheritEnv(cachedConfig)
		return cachedConfig, configError
	}

	rr := &remoteResolver{
		readRemotes: git.Remotes,
		getConfig:   configFunc,
	}
	remotesFunc := rr.Resolver()

	return &cmdutil.Factory{
		IOStreams: io,
		Config:    configFunc,
		Remotes:   remotesFunc,
		HttpClient: func() (*http.Client, error) {
			cfg, err := configFunc()
			if err != nil {
				return nil, err
			}

			return NewHTTPClient(io, cfg, appVersion, true), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			remotes, err := remotesFunc()
			if err != nil {
				return nil, err
			}
			return remotes[0], nil
		},
		Branch: func() (string, error) {
			currentBranch, err := git.CurrentBranch()
			if err != nil {
				return "", fmt.Errorf("could not determine current branch: %w", err)
			}
			return currentBranch, nil
		},
	}
}
