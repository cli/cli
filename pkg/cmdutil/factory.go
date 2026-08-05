package cmdutil

import (
	"net/http"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
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
	// GitHubREST returns a GitHub REST API client for host, authenticated as
	// that host's active user.
	//
	// It takes the host rather than being a plain func() because the host is
	// usually not known until a command has resolved BaseRepo or read a
	// --host flag, and a few commands span hosts. Asking for a client at the
	// moment the host is known is what lets the token be settled once, at
	// construction, instead of being filled in per request by a transport.
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
	// GitHubRESTAnonymous returns a GitHub REST API client for host that sends
	// no token of its own.
	//
	// This is for requests that must not carry the user's credentials, such as
	// the release check in internal/update.
	GitHubRESTAnonymous func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
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
