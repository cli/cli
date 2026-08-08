package cmdutil

import (
	"net/http"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type Factory struct {
	AppVersion     string
	ExecutablePath string
	InvokingAgent  string

	Browser          browser.Browser
	ExtensionManager extensions.ExtensionManager
	GitClient        *git.Client
	IOStreams        *iostreams.IOStreams
	Prompter         prompter.Prompter

	BaseRepo func() (ghrepo.Interface, error)
	Branch   func() (string, error)
	// It would be nice if Config were just loaded once at startup and an error
	// were returned, but this would prevent commands like "gh version" from running.
	// So for now, we eagerly load the config and don't fail if there is an error,
	// and defer the error handling to commands that need it.
	// HOWEVER, as an additional point, the root command setup currently DOES call
	// this and errors, so we never get to "gh version" anyway.
	// We need to revisit that, but I don't want to make it worse.
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)
	// PlainHttpClient is a special HTTP client that does not automatically set
	// auth and other headers. This is meant to be used in situations where the
	// client needs to specify the headers itself (e.g. during login).
	PlainHttpClient func() (*http.Client, error)
	// ExternalHttpClient is an HTTP client for talking to non-GitHub hosts
	// It includes debug logging and a User-Agent header but does not attach any
	// authentication tokens or GitHub-specific headers.
	ExternalHttpClient func() (*http.Client, error)
	Remotes            func() (context.Remotes, error)
}
