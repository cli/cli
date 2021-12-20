package factory

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/extension"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func New(appVersion string) *cmdutil.Factory {
	var exe string
	f := &cmdutil.Factory{
		Config: configFunc(), // No factory dependencies
		Branch: branchFunc(), // No factory dependencies
		Executable: func() string {
			if exe != "" {
				return exe
			}
			exe = executable("gh")
			return exe
		},
	}

	f.IOStreams = ioStreams(f)                   // Depends on Config
	f.HttpClient = httpClientFunc(f, appVersion) // Depends on Config, IOStreams, and appVersion
	f.Remotes = remotesFunc(f)                   // Depends on Config
	f.BaseRepo = BaseRepoFunc(f)                 // Depends on Remotes
	f.Browser = browser(f)                       // Depends on Config, and IOStreams
	f.ExtensionManager = extensionManager(f)     // Depends on Config, HttpClient, and IOStreams

	return f
}

func BaseRepoFunc(f *cmdutil.Factory) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		remotes, err := f.Remotes()
		if err != nil {
			return nil, err
		}
		return remotes[0], nil
	}
}

func SmartBaseRepoFunc(f *cmdutil.Factory) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		httpClient, err := f.HttpClient()
		if err != nil {
			return nil, err
		}

		apiClient := api.NewClientFromHTTP(httpClient)

		remotes, err := f.Remotes()
		if err != nil {
			return nil, err
		}
		repoContext, err := context.ResolveRemotesToRepos(remotes, apiClient, "")
		if err != nil {
			return nil, err
		}
		baseRepo, err := repoContext.BaseRepo(f.IOStreams)
		if err != nil {
			return nil, err
		}

		return baseRepo, nil
	}
}

func remotesFunc(f *cmdutil.Factory) func() (context.Remotes, error) {
	rr := &remoteResolver{
		readRemotes: git.Remotes,
		getConfig:   f.Config,
	}
	return rr.Resolver()
}

func httpClientFunc(f *cmdutil.Factory, appVersion string) func() (*http.Client, error) {
	return func() (*http.Client, error) {
		io := f.IOStreams
		cfg, err := f.Config()
		if err != nil {
			return nil, err
		}
		return NewHTTPClient(io, cfg, appVersion, true)
	}
}

func browser(f *cmdutil.Factory) cmdutil.Browser {
	io := f.IOStreams
	return cmdutil.NewBrowser(browserLauncher(f), io.Out, io.ErrOut)
}

// Browser precedence
// 1. GH_BROWSER
// 2. browser from config
// 3. BROWSER
func browserLauncher(f *cmdutil.Factory) string {
	if ghBrowser := os.Getenv("GH_BROWSER"); ghBrowser != "" {
		return ghBrowser
	}

	cfg, err := f.Config()
	if err == nil {
		if cfgBrowser, _ := cfg.Get("", "browser"); cfgBrowser != "" {
			return cfgBrowser
		}
	}

	return os.Getenv("BROWSER")
}

// Finds the location of the executable for the current process as it's found in PATH, respecting symlinks.
// If the process couldn't determine its location, return fallbackName. If the executable wasn't found in
// PATH, return the absolute location to the program.
//
// The idea is that the result of this function is callable in the future and refers to the same
// installation of gh, even across upgrades. This is needed primarily for Homebrew, which installs software
// under a location such as `/usr/local/Cellar/gh/1.13.1/bin/gh` and symlinks it from `/usr/local/bin/gh`.
// When the version is upgraded, Homebrew will often delete older versions, but keep the symlink. Because of
// this, we want to refer to the `gh` binary as `/usr/local/bin/gh` and not as its internal Homebrew
// location.
//
// None of this would be needed if we could just refer to GitHub CLI as `gh`, i.e. without using an absolute
// path. However, for some reason Homebrew does not include `/usr/local/bin` in PATH when it invokes git
// commands to update its taps. If `gh` (no path) is being used as git credential helper, as set up by `gh
// auth login`, running `brew update` will print out authentication errors as git is unable to locate
// Homebrew-installed `gh`.
func executable(fallbackName string) string {
	exe, err := os.Executable()
	if err != nil {
		return fallbackName
	}

	base := filepath.Base(exe)
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		p, err := filepath.Abs(filepath.Join(dir, base))
		if err != nil {
			continue
		}
		f, err := os.Stat(p)
		if err != nil {
			continue
		}

		if p == exe {
			return p
		} else if f.Mode()&os.ModeSymlink != 0 {
			if t, err := os.Readlink(p); err == nil && t == exe {
				return p
			}
		}
	}

	return exe
}

func configFunc() func() (config.Config, error) {
	var cachedConfig config.Config
	var configError error
	return func() (config.Config, error) {
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
}

func branchFunc() func() (string, error) {
	return func() (string, error) {
		currentBranch, err := git.CurrentBranch()
		if err != nil {
			return "", fmt.Errorf("could not determine current branch: %w", err)
		}
		return currentBranch, nil
	}
}

func extensionManager(f *cmdutil.Factory) *extension.Manager {
	em := extension.NewManager(f.IOStreams)

	cfg, err := f.Config()
	if err != nil {
		return em
	}
	em.SetConfig(cfg)

	client, err := f.HttpClient()
	if err != nil {
		return em
	}

	em.SetClient(api.NewCachedClient(client, time.Second*30))

	return em
}

func ioStreams(f *cmdutil.Factory) *iostreams.IOStreams {
	io := iostreams.System()
	cfg, err := f.Config()
	if err != nil {
		return io
	}

	if prompt, _ := cfg.Get("", "prompt"); prompt == "disabled" {
		io.SetNeverPrompt(true)
	}

	// Pager precedence
	// 1. GH_PAGER
	// 2. pager from config
	// 3. PAGER
	if ghPager, ghPagerExists := os.LookupEnv("GH_PAGER"); ghPagerExists {
		io.SetPager(ghPager)
	} else if pager, _ := cfg.Get("", "pager"); pager != "" {
		io.SetPager(pager)
	}

	return io
}
