package status

import (
	"net/http"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/iostreams"
)

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
}
