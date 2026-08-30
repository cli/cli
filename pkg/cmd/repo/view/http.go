package view

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/go-gh/v2/pkg/asciisanitizer"
	"golang.org/x/text/transform"
)

var NotFoundError = errors.New("not found")

type RepoReadme struct {
	Filename string
	Content  string
	BaseURL  string
}

func RepositoryReadme(client *http.Client, repo ghrepo.Interface, branch string) (*RepoReadme, error) {
	apiClient := api.NewClientFromHTTP(client)
	var response struct {
		Name    string
		Content string
		HTMLURL string `json:"html_url"`
	}

	readmePath, err := getReadmePath(repo, branch)
	if err != nil {
		return nil, err
	}

	err = apiClient.REST(repo.RepoHost(), "GET", readmePath.String(), nil, &response)
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

	sanitized, err := io.ReadAll(transform.NewReader(bytes.NewReader(decoded), &asciisanitizer.Sanitizer{}))
	if err != nil {
		return nil, err
	}

	return &RepoReadme{
		Filename: response.Name,
		Content:  string(sanitized),
		BaseURL:  response.HTMLURL,
	}, nil
}

func getReadmePath(repo ghrepo.Interface, branch string) (safeurl.SafeURL, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "readme")
	if err != nil {
		return nil, err
	}
	if branch != "" {
		path.SetQuery("ref", branch)
	}
	return path, nil
}
