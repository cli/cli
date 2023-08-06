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
		repoContext, err := ghContext.ResolveRemotesToRepos(remotes, apiClient, "")
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

func configFunc() func() (config.Config, error) {
	var cachedConfig config.Config
	var configError error
	return func() (config.Config, error) {
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
	} else if prompt, _ := cfg.GetOrDefault("", "prompt"); prompt == "disabled" {
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
