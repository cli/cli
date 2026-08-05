package api

import (
	"context"
	"errors"
	"net/http"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
)

const (
	apiVersion      = "X-GitHub-Api-Version"
	apiVersionValue = "2022-11-28"
	authorization   = "Authorization"
	cacheTTL        = "X-GH-CACHE-TTL"
	graphqlFeatures = "GraphQL-Features"
	features        = "merge_queue"
	userAgent       = "User-Agent"
)

func NewClientFromHTTP(httpClient *http.Client) *Client {
	client := &Client{http: httpClient}
	return client
}

type Client struct {
	http *http.Client
}

func (c *Client) HTTP() *http.Client {
	return c.http
}

type GraphQLError struct {
	*ghAPI.GraphQLError
}

// GraphQL performs a GraphQL request using the query string and parses the response into data receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) GraphQL(hostname string, query string, variables map[string]interface{}, data interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Do(query, variables, data))
}

// Mutate performs a GraphQL mutation based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) Mutate(hostname, name string, mutation interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Mutate(name, mutation, variables))
}

// Query performs a GraphQL query based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) Query(hostname, name string, query interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Query(name, query, variables))
}

// QueryWithContext performs a GraphQL query based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) QueryWithContext(ctx context.Context, hostname, name string, query interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.QueryWithContext(ctx, name, query, variables))
}

// handleResponse converts a ghAPI.GraphQLError into this package's GraphQLError.
//
// It no longer has a REST leg: every REST request goes through
// internal/githubrest, which builds its own errors.
func handleResponse(err error) error {
	if err == nil {
		return nil
	}

	var gqlErr *ghAPI.GraphQLError
	if errors.As(err, &gqlErr) {
		return GraphQLError{
			GraphQLError: gqlErr,
		}
	}

	return err
}

func clientOptions(hostname string, transport http.RoundTripper) ghAPI.ClientOptions {
	// AuthToken, and Headers are being handled by transport,
	// so let go-gh know that it does not need to resolve them.
	opts := ghAPI.ClientOptions{
		AuthToken: "none",
		Headers: map[string]string{
			authorization: "",
			apiVersion:    apiVersionValue,
		},
		Host:               hostname,
		SkipDefaultHeaders: true,
		Transport:          transport,
		LogIgnoreEnv:       true,
	}
	return opts
}
