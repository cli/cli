package view

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
)

type GistFile struct {
	Filename string
	Type     string
	Language string
	Content  string
}

type Gist struct {
	Description string
	Files       map[string]GistFile
}

func getGist(client *http.Client, hostname, gistID string) (*Gist, error) {
	gist := Gist{}
	path := fmt.Sprintf("gists/%s", gistID)

	apiClient := api.NewClientFromHTTP(client)
	err := apiClient.REST(hostname, "GET", path, nil, &gist)
	if err != nil {
		return nil, err
	}

	return &gist, nil
}
