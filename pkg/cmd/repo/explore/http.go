package explore

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

var NotFoundError = errors.New("not found")

type RepoTree []RepoTreeNode

type RepoTreeNode struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	SHA  string `json:"sha"`
	URL  string `json:"url"`
}

func (rtn *RepoTreeNode) IsDir() bool {
	return rtn.Type == "tree"
}

func (rtn *RepoTreeNode) Name() string {
	return filepath.Base(rtn.Path)
}

func (rtn *RepoTreeNode) Dir() string {
	return filepath.Dir(rtn.Path)
}

func (rtn *RepoTreeNode) Ext() string {
	return filepath.Ext(rtn.Path)
}

func repositoryTree(client *api.Client, repo ghrepo.Interface, branch string) (RepoTree, error) {
	var response struct {
		SHA       string   `json:"sha"`
		URL       string   `json:"url"`
		Tree      RepoTree `json:"tree"`
		Truncated bool     `json:"truncated"`
	}

	err := client.REST(repo.RepoHost(), "GET", repositoryTreePath(repo, branch), nil, &response)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && httpError.StatusCode == 404 {
			return nil, NotFoundError
		}
		return nil, err
	}

	return response.Tree, nil
}

// https://docs.github.com/en/rest/reference/git#get-a-tree
func repositoryTreePath(repo ghrepo.Interface, branch string) string {
	return fmt.Sprintf("repos/%s/%s/git/trees/%s?recursive=1", repo.RepoOwner(), repo.RepoName(), branch)
}

func repositoryFileContent(client *api.Client, repo ghrepo.Interface, ref, filePath string) ([]byte, error) {
	path := fmt.Sprintf("repos/%s/contents/%s", ghrepo.FullName(repo), filePath)
	if ref != "" {
		q := fmt.Sprintf("?ref=%s", url.QueryEscape(ref))
		path = path + q
	}

	type Result struct {
		Content string `json:"content"`
	}

	var result Result
	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode file: %w", err)
	}

	return decoded, nil
}
