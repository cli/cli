package client

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
)

func ValidateHostCfg(hostname string, cfg config.Config) error {
	apiClient, err := ClientFromCfg(hostname, cfg)
	if err != nil {
		return err
	}

	err = apiClient.HasMinimumScopes(hostname)
	if err != nil {
		return fmt.Errorf("could not validate token: %w", err)
	}

	return nil
}

var ClientFromCfg = func(hostname string, cfg config.Config) (*api.Client, error) {
	var opts []api.ClientOption

	token, err := cfg.Get(hostname, "oauth_token")
	if err != nil {
		return nil, err
	}

	if token == "" {
		return nil, fmt.Errorf("no token found in config for %s", hostname)
	}

	opts = append(opts,
		// no access to Version so the user agent is more generic here.
		api.AddHeader("User-Agent", "GitHub CLI"),
		api.AddHeaderFunc("Authorization", func(req *http.Request) (string, error) {
			return fmt.Sprintf("token %s", token), nil
		}),
	)

	httpClient := api.NewHTTPClient(opts...)

	return api.NewClientFromHTTP(httpClient), nil
}
