package httpmock

import (
	"net/http"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/githubrest"
)

// RESTClientFunc returns a stand-in for cmdutil.Factory.GitHubREST that serves
// every host from reg.
//
// Command tests set Options.GitHubREST to this rather than building a client by
// hand, because the construction is identical everywhere and repeating it puts
// base URL resolution into hundreds of test files, where a drift from the real
// factory would go unnoticed.
//
// The token is a fixed placeholder. A test that cares which credentials are
// sent should assert on the request's Authorization header, and one that needs
// a particular token should build its own client.
func RESTClientFunc(reg *Registry) func(string, ...githubrest.ClientOption) (*githubrest.Client, error) {
	return restClientFunc(reg, githubrest.WithToken("test-token"))
}

// RESTClientFuncAnonymous is RESTClientFunc for
// cmdutil.Factory.GitHubRESTAnonymous, sending no token.
func RESTClientFuncAnonymous(reg *Registry) func(string, ...githubrest.ClientOption) (*githubrest.Client, error) {
	return restClientFunc(reg, githubrest.WithoutToken())
}

func restClientFunc(reg *Registry, auth githubrest.AuthStrategy) func(string, ...githubrest.ClientOption) (*githubrest.Client, error) {
	return func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error) {
		opts = append([]githubrest.ClientOption{
			githubrest.WithCredentialedHost(ghinstance.UploadHost(host)),
		}, opts...)
		return githubrest.NewClient(ghinstance.RESTPrefix(host), &http.Client{Transport: reg}, auth, opts...)
	}
}
