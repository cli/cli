package cmdutil

import (
	"net/http"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/pkg/api"
)

type Browser interface {
	Browse(string) error
}

type Factory struct {
	IOStreams *iostreams.IOStreams
	Browser   Browser

	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Config     func() (config.Config, error)
	Branch     func() (string, error)

	ExtensionManager extensions.ExtensionManager

	// Executable is the path to the currently invoked gh binary
	Executable func() string

	RESTClient        func(*api.ClientOptions) (api.RESTClient, error)
	GQLClient         func(*api.ClientOptions) (api.GQLClient, error)
	CurrentRepository func() (ghrepo.Interface, error)
}
