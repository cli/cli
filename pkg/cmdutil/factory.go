package cmdutil

import (
	"net/http"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/iostreams"
)

type Factory struct {
	IOStreams        *iostreams.IOStreams
	HttpClient       func() (*http.Client, error)
	ResolvedBaseRepo func(*http.Client) (ghrepo.Interface, error)
	BaseRepo         func() (ghrepo.Interface, error)
}
