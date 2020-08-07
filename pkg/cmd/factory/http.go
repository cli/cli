package factory

import (
	"errors"
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
func httpClient(io *iostreams.IOStreams, cfg config.Config, appVersion string, setAccept bool) *http.Client {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		logTraffic := strings.Contains(verbose, "api")
		opts = append(opts, api.VerboseLog(io.ErrOut, logTraffic, io.IsStderrTTY()))
	}

	opts = append(opts,
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", appVersion)),
		api.AddHeaderFunc("Authorization", func(req *http.Request) (string, error) {
			if token := os.Getenv("GITHUB_TOKEN"); token != "" {
				return fmt.Sprintf("token %s", token), nil
			}

			hostname := ghinstance.NormalizeHostname(req.URL.Hostname())
			token, err := cfg.Get(hostname, "oauth_token")
			if token == "" {
				var notFound *config.NotFoundError
				// TODO: check if stdout is TTY too
				if errors.As(err, &notFound) && io.IsStdinTTY() {
					// interactive OAuth flow
					token, err = config.AuthFlowWithConfig(cfg, hostname, "Notice: authentication required")
				}
				if err != nil {
					return "", err
				}
				if token == "" {
					// TODO: instruct user how to manually authenticate
					return "", fmt.Errorf("authentication required for %s", hostname)
				}
			}

			return fmt.Sprintf("token %s", token), nil
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
