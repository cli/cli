package factory

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/iostreams"
)

// generic authenticated HTTP client for commands
func NewHTTPClient(io *iostreams.IOStreams, cfg config.Config, appVersion string, setAccept bool) *http.Client {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		logTraffic := strings.Contains(verbose, "api")
		opts = append(opts, api.VerboseLog(io.ErrOut, logTraffic, io.IsStderrTTY()))
	}

	opts = append(opts,
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", appVersion)),
		api.AddHeaderFunc("Authorization", func(req *http.Request) (string, error) {
			hostname := ghinstance.NormalizeHostname(req.URL.Hostname())
			if token, err := cfg.Get(hostname, "oauth_token"); err == nil && token != "" {
				return fmt.Sprintf("token %s", token), nil
			}
			return "", nil
		}),
	)

	if setAccept {
		opts = append(opts,
			api.AddHeaderFunc("Accept", func(req *http.Request) (string, error) {
				// antiope-preview: Checks
				accept := "application/vnd.github.antiope-preview+json"
				if ghinstance.IsEnterprise(req.URL.Hostname()) {
					// shadow-cat-preview: Draft pull requests
					accept += ", application/vnd.github.shadow-cat-preview"
				}
				return accept, nil
			}),
		)
	}

	return api.NewHTTPClient(opts...)
}
