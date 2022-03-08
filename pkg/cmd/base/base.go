package base

import (
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"

	"github.com/cli/cli/v2/api"
)

func GetAllResolvedRemotes(Remotes func() (context.Remotes, error), HttpClient func() (*http.Client, error)) ([]func(ghrepo.Interface, error), error) {
	// return []func() (ghrepo.Interface, error) {
	httpClient, err := HttpClient()
	if err != nil {
		return nil, err
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	remotes, err := Remotes()
	if err != nil {
		return nil, err
	}

	repoContext, err := context.ResolveRemotesToRepos(remotes, apiClient, "")
	if err != nil {
		return nil, err
	}

	repoContext.GetBaseRepo()
	// 	baseRepo, err := repoContext.BaseRepo(f.IOStreams)
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	return baseRepo, nil
	// }
	// urn nil
}
