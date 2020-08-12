package factory

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/cli/cli/context"
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
		return cachedConfig, configError
	}

	remotesFunc := remotesResolver()

	return &cmdutil.Factory{
		IOStreams: io,
		Config:    configFunc,
		Remotes:   remotesFunc,
		HttpClient: func() (*http.Client, error) {
			cfg, err := configFunc()
			if err != nil {
				return nil, err
			}

			// TODO: avoid setting Accept header for `api` command
			return httpClient(io, cfg, appVersion, true), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			remotes, err := remotesFunc()
			if err != nil {
				return nil, err
			}
			return remotes.FindByName("upstream", "github", "origin", "*")
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

// TODO: pass in a Config instance to parse remotes based on pre-authenticated hostnames
func remotesResolver() func() (context.Remotes, error) {
	var cachedRemotes context.Remotes
	var remotesError error

	return func() (context.Remotes, error) {
		if cachedRemotes != nil || remotesError != nil {
			return cachedRemotes, remotesError
		}

		gitRemotes, err := git.Remotes()
		if err != nil {
			remotesError = err
			return nil, err
		}
		if len(gitRemotes) == 0 {
			remotesError = errors.New("no git remotes found")
			return nil, remotesError
		}

		sshTranslate := git.ParseSSHConfig().Translator()
		resolvedRemotes := context.TranslateRemotes(gitRemotes, sshTranslate)

		// determine hostname by looking at the primary remotes
		var hostname string
		if mainRemote, err := resolvedRemotes.FindByName("upstream", "github", "origin", "*"); err == nil {
			hostname = mainRemote.RepoHost()
		}

		// filter the rest of the remotes to just that hostname
		cachedRemotes = context.Remotes{}
		for _, r := range resolvedRemotes {
			if r.RepoHost() != hostname {
				continue
			}
			cachedRemotes = append(cachedRemotes, r)
		}
		return cachedRemotes, nil
	}
}
