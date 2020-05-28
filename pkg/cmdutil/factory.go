package cmdutil

import (
	"net/http"

	"github.com/cli/cli/pkg/iostreams"
)

type Factory struct {
	IOStreams  *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
}
