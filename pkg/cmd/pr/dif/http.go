package dif

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func getFileContent(cache Cache, httpClient *http.Client, repo ghrepo.Interface, file, ref string) (string, error) {
	filepath := filepath.Join(os.TempDir(), "gh-cli-cache", ref, file)
	if !cache.Exists(filepath) {
		client := api.NewClientFromHTTP(httpClient)
		path := fmt.Sprintf("repos/%s/contents/%s", ghrepo.FullName(repo), file)
		if ref != "" {
			q := fmt.Sprintf("?ref=%s", url.QueryEscape(ref))
			path = path + q
		}
		type Result struct {
			Content string
		}
		var result Result
		err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
		if err != nil {
			return "", err
		}
		decoded, err := base64.StdEncoding.DecodeString(result.Content)
		if err != nil {
			return "", err
		}
		err = cache.Create(filepath, decoded)
		if err != nil {
			return "", err
		}
	}
	return filepath, nil
}
