package shared

import (
	"github.com/cli/cli/v2/pkg/cmd/event/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

// EventClientFunc returns a function that builds an event client for a host, using the factory's HTTP client.
func EventClientFunc(f *cmdutil.Factory) func(host string) (client.EventClient, error) {
	return func(host string) (client.EventClient, error) {
		httpClient, err := f.HttpClient()
		if err != nil {
			return nil, err
		}
		return client.NewEventClient(httpClient, host), nil
	}
}
