package login

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
)

var validateToken = func(hostname, token string) error {
	var opts []api.ClientOption

	opts = append(opts,
		// TODO we don't have access to Version. Is it worth the gymnastics to get that value in here?
		// api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)),
		api.AddHeaderFunc("Authorization", func(req *http.Request) (string, error) {
			return fmt.Sprintf("token %s", token), nil
		}),
	)

	httpClient := api.NewHTTPClient(opts...)
	apiClient := api.NewClientFromHTTP(httpClient)

	_, err := apiClient.HasMinimumScopes(hostname)
	if err != nil {
		return fmt.Errorf("could not validate token: %w", err)
	}

	return nil
}
