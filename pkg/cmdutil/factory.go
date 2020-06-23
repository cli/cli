package cmdutil

import (
	"io"
	"net/http"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/iostreams"
)

type Factory struct {
	IOStreams    *iostreams.IOStreams
	HttpClient   func() (*http.Client, error)
	BaseRepo     func() (ghrepo.Interface, error)
	ColorableErr io.Writer
	ColorableOut io.Writer
}
