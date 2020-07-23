package view

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

var NotFoundError = errors.New("not found")

type RepoReadme struct {
	Filename string
	Content  string
}

func RepositoryReadme(client *http.Client, repo ghrepo.Interface) (*RepoReadme, error) {
	apiClient := api.NewClientFromHTTP(client)
	var response struct {
		Name    string
		Content string
	}

	err := apiClient.REST("GET", fmt.Sprintf("repos/%s/readme", ghrepo.FullName(repo)), nil, &response)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && httpError.StatusCode == 404 {
			return nil, NotFoundError
		}
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(response.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode readme: %w", err)
	}

	return &RepoReadme{
		Filename: response.Name,
		Content:  string(decoded),
	}, nil
}
