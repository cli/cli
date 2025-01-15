package factory

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/extension"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

var ssoHeader string
var ssoURLRE = regexp.MustCompile(`\burl=([^;]+)`)

func New(appVersion string) *cmdutil.Factory {
	f := &cmdutil.Factory{
		AppVersion:     appVersion,
		Config:         configFunc(), // No factory dependencies
		ExecutableName: "gh",
	}

	f.IOStreams = ioStreams(f)                   // Depends on Config
	f.HttpClient = httpClientFunc(f, appVersion) // Depends on Config, IOStreams, and appVersion
	f.GitClient = newGitClient(f)                // Depends on IOStreams, and Executable
	f.Remotes = remotesFunc(f)                   // Depends on Config, and GitClient
	f.BaseRepo = BaseRepoFunc(f)                 // Depends on Remotes
	f.Prompter = newPrompter(f)                  // Depends on Config and IOStreams
	f.Browser = newBrowser(f)                    // Depends on Config, and IOStreams
	f.ExtensionManager = extensionManager(f)     // Depends on Config, HttpClient, and IOStreams
	f.Branch = branchFunc(f)                     // Depends on GitClient

	return f
}

// BaseRepoFunc requests a list of Remotes, and selects the first one.
// Although Remotes is injected via the factory so it looks like the function might
// be configurable, in practice, it's calling readRemotes, and the injection is indirection.
//
// readRemotes makes use of the remoteResolver, which is responsible for requesting the list
// of remotes for the current working directory from git. It then does some filtering to
// only retain remotes for hosts that we have authenticated against; keep in mind this may
// be the single value of GH_HOST.
//
// That list of remotes is sorted by their remote name, in the following order:
//  1. upstream
//  2. github
//  3. origin
//  4. other remotes, no ordering guaratanteed because the sort function is not stable
//
// Given that list, this function chooses the first one.
//
// Here's a common example of when this might matter: when we clone a fork, by default we add
// the parent as a remote named upstream. So the remotes may look like this:
// upstream  https://github.com/cli/cli.git (fetch)
// upstream https://github.com/cli/cli.git (push)
// origin  https://github.com/cli/cli-fork.git (fetch)
// origin  https://github.com/cli/cli-fork.git (push)
//
// With this resolution function, the upstream will always be chosen (assuming we have authenticated with github.com).
func BaseRepoFunc(f *cmdutil.Factory) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		remotes, err := f.Remotes()
		if err != nil {
			return nil, err
		}
		return remotes[0], nil
	}
}

// SmartBaseRepoFunc provides additional behaviour over BaseRepoFunc. Read the BaseRepoFunc
// documentation for more information on how remotes are fetched and ordered.
//
// Unlike BaseRepoFunc, instead of selecting the first remote in the list, this function will
// use the API to resolve repository networks, and attempt to use the `resolved` git remote config value
// as part of determining the base repository.
//
// Although the behaviour commented below really belongs to the `BaseRepo` function on `ResolvedRemotes`,
// in practice the most important place to understand the general behaviour is here, so that's where
// I'm going to write it.
//
// Firstly, the remotes are inspected to see whether any are already resolved. Resolution means the git
// config value of the `resolved` key was `base` (meaning this remote is the base repository), or a specific
// repository e.g. `cli/cli` (meaning that specific repo is the base repo, regardless of whether a remote
// exists for it). These values are set by default on clone of a fork, or by running `repo set-default`. If
// either are set, that repository is returned.
//
// If we the current invocation is unable to prompt, then the first remote is returned. I believe this behaviour
// exists for backwards compatibility before the later steps were introduced, however, this is frequently a source
// of differing behaviour between interactive and non-interactive invocations:
//
// ➜ git remote -v
// origin  https://github.com/williammartin/test-repo.git (fetch)
// origin  https://github.com/williammartin/test-repo.git (push)
// upstream        https://github.com/williammartin-test-org/test-repo.git (fetch)
// upstream        https://github.com/williammartin-test-org/test-repo.git (push)
//
// ➜ gh pr list
// X No default remote repository has been set for this directory.
//
// please run `gh repo set-default` to select a default remote repository.
// ➜ gh pr list | cat
// 3       test    williammartin-test-org:remote-push-default-feature      OPEN    2024-12-13T10:28:40Z
//
// Furthermore, when repositories have been renamed on the server and not on the local git remote, this causes
// even more confusion because the API requests can be different, and FURTHERMORE this can be an issue for
// services that don't handle renames correctly, like the ElasticSearch indexing.
//
// Assuming we have an interactive invocation, then the next step is to resolve a network of respositories. This
// involves creating a dynamic GQL query requesting information about each repository (up to a limit of 5).
// Each returned repo is added to a list, along with its parent, if present in the query response.
// The repositories in the query retain the same ordering as previously outlined. Interestingly, the request is sent
// to the hostname of the first repo, so if you happen to have remotes on different GitHub hosts, then they won't
// resolve correctly. I'm not sure this has ever caused an issue, but does seem like a potential source of bugs.
// In practice, since the remotes are ordered with upstream, github, origin before others, it's almost always going
// to be the case that the correct host is chosen.
//
// Because fetching the network includes the parent repo, even if it is not a remote, this requires the user to
// disambiguate, which can be surprising, though I'm not sure I've heard anyone complain:
//
// ➜ git remote -v
// origin  https://github.com/williammartin/test-repo.git (fetch)
// origin  https://github.com/williammartin/test-repo.git (push)
//
// ➜ gh pr list
// X No default remote repository has been set for this directory.
//
// please run `gh repo set-default` to select a default remote repository.
//
// If no repos are returned from the API then we return the first remote from the original list. I'm not sure
// why we do this rather than erroring, because it seems like almost every future step is going to fail when hitting
// the API. Potentially it helps if there is an API blip? It was added without comment in:
// https://github.com/cli/cli/pull/1706/files#diff-65730f0373fb91dd749940cf09daeaf884e5643d665a6c3eb09d54785a6d475eR113
//
// If one repo is returned from the API, then that one is returned as the base repo.
//
// If more than one repo is returned from the API, we indicate to the user that they need to run `repo set-default`,
// and return an error with no base repo.
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
		resolvedRepos, err := ghContext.ResolveRemotesToRepos(remotes, apiClient, "")
		if err != nil {
			return nil, err
		}
		baseRepo, err := resolvedRepos.BaseRepo(f.IOStreams)
		if err != nil {
			return nil, err
		}

		return baseRepo, nil
	}
}

func remotesFunc(f *cmdutil.Factory) func() (ghContext.Remotes, error) {
	rr := &remoteResolver{
		readRemotes: func() (git.RemoteSet, error) {
			return f.GitClient.Remotes(context.Background())
		},
		getConfig: f.Config,
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
		opts := api.HTTPClientOptions{
			Config:      cfg.Authentication(),
			Log:         io.ErrOut,
			LogColorize: io.ColorEnabled(),
			AppVersion:  appVersion,
		}
		client, err := api.NewHTTPClient(opts)
		if err != nil {
			return nil, err
		}
		client.Transport = api.ExtractHeader("X-GitHub-SSO", &ssoHeader)(client.Transport)
		return client, nil
	}
}

func newGitClient(f *cmdutil.Factory) *git.Client {
	io := f.IOStreams
	ghPath := f.Executable()
	client := &git.Client{
		GhPath: ghPath,
		Stderr: io.ErrOut,
		Stdin:  io.In,
		Stdout: io.Out,
	}
	return client
}

func newBrowser(f *cmdutil.Factory) browser.Browser {
	io := f.IOStreams
	return browser.New("", io.Out, io.ErrOut)
}

func newPrompter(f *cmdutil.Factory) prompter.Prompter {
	editor, _ := cmdutil.DetermineEditor(f.Config)
	io := f.IOStreams
	return prompter.New(editor, io.In, io.Out, io.ErrOut)
}

func configFunc() func() (gh.Config, error) {
	var cachedConfig gh.Config
	var configError error
	return func() (gh.Config, error) {
		if cachedConfig != nil || configError != nil {
			return cachedConfig, configError
		}
		cachedConfig, configError = config.NewConfig()
		return cachedConfig, configError
	}
}

func branchFunc(f *cmdutil.Factory) func() (string, error) {
	return func() (string, error) {
		currentBranch, err := f.GitClient.CurrentBranch(context.Background())
		if err != nil {
			return "", fmt.Errorf("could not determine current branch: %w", err)
		}
		return currentBranch, nil
	}
}

func extensionManager(f *cmdutil.Factory) *extension.Manager {
	em := extension.NewManager(f.IOStreams, f.GitClient)

	cfg, err := f.Config()
	if err != nil {
		return em
	}
	em.SetConfig(cfg)

	client, err := f.HttpClient()
	if err != nil {
		return em
	}

	em.SetClient(api.NewCachedHTTPClient(client, time.Second*30))

	return em
}

func ioStreams(f *cmdutil.Factory) *iostreams.IOStreams {
	io := iostreams.System()
	cfg, err := f.Config()
	if err != nil {
		return io
	}

	if _, ghPromptDisabled := os.LookupEnv("GH_PROMPT_DISABLED"); ghPromptDisabled {
		io.SetNeverPrompt(true)
	} else if prompt := cfg.Prompt(""); prompt.Value == "disabled" {
		io.SetNeverPrompt(true)
	}

	// Pager precedence
	// 1. GH_PAGER
	// 2. pager from config
	// 3. PAGER
	if ghPager, ghPagerExists := os.LookupEnv("GH_PAGER"); ghPagerExists {
		io.SetPager(ghPager)
	} else if pager := cfg.Pager(""); pager.Value != "" {
		io.SetPager(pager.Value)
	}

	return io
}

// SSOURL returns the URL of a SAML SSO challenge received by the server for clients that use ExtractHeader
// to extract the value of the "X-GitHub-SSO" response header.
func SSOURL() string {
	if ssoHeader == "" {
		return ""
	}
	m := ssoURLRE.FindStringSubmatch(ssoHeader)
	if m == nil {
		return ""
	}
	return m[1]
}
