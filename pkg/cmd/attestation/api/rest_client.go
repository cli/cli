package api

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cli/cli/v2/internal/githubrest"
)

// restAPIClient adapts a githubrest.Client to the githubApiClient interface
// this package already retries and mocks against.
//
// The hostname argument is ignored: the client is built for one host at
// construction. It stays in the signature because the interface is what the
// package's test doubles implement.
type restAPIClient struct {
	ctx    context.Context
	client *githubrest.Client
}

func (c restAPIClient) REST(hostname, method, p string, body io.Reader, data interface{}) error {
	_, err := c.RESTWithNext(hostname, method, p, body, data)
	return err
}

func (c restAPIClient) RESTWithNext(hostname, method, p string, body io.Reader, data interface{}) (string, error) {
	req, err := c.client.NewRequest(c.ctx, method, p, body)
	if err != nil {
		return "", err
	}

	// Send rather than Do, because attestations are paginated by Link header.
	resp, err := c.client.Send(req)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return "", err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(data); err != nil {
		return "", err
	}

	return resp.NextPage(), nil
}
