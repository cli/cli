package client

import (
	"os"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

func New(f *cmdutil.Factory) (*queries.Client, error) {
	if f.HttpClient == nil {
		// This is for compatibility with tests that exercise Cobra command functionality.
		// These tests do not define a `HttpClient` nor do they need to.
		return nil, nil
	}

	httpClient, err := f.HttpClient()
	if err != nil {
		return nil, err
	}
	return queries.NewClient(httpClient, os.Getenv("GH_HOST"), f.IOStreams), nil
}
