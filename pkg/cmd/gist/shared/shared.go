package shared

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/cli/api"
)

type GistFile struct {
	Filename string `json:"filename,omitempty"`
	Type     string `json:"type,omitempty"`
	Language string `json:"language,omitempty"`
	Content  string `json:"content,omitempty"`
}

type GistOwner struct {
	Login string `json:"login,omitempty"`
}

type Gist struct {
	ID          string               `json:"id,omitempty"`
	Description string               `json:"description"`
	Files       map[string]*GistFile `json:"files"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Public      bool                 `json:"public"`
	HTMLURL     string               `json:"html_url,omitempty"`
	Owner       *GistOwner           `json:"owner,omitempty"`
}

func GetGist(client *http.Client, hostname, gistID string) (*Gist, error) {
	gist := Gist{}
	path := fmt.Sprintf("gists/%s", gistID)

	apiClient := api.NewClientFromHTTP(client)
	err := apiClient.REST(hostname, "GET", path, nil, &gist)
	if err != nil {
		return nil, err
	}

	return &gist, nil
}

func GistIDFromURL(gistURL string) (string, error) {
	u, err := url.Parse(gistURL)
	if err == nil && strings.HasPrefix(u.Path, "/") {
		split := strings.Split(u.Path, "/")

		if len(split) > 2 {
			return split[2], nil
		}

		if len(split) == 2 && split[1] != "" {
			return split[1], nil
		}
	}

	return "", fmt.Errorf("Invalid gist URL %s", u)
}
