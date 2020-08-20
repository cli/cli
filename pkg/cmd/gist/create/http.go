package create

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/cli/cli/api"
)

// Gist represents a GitHub's gist.
type Gist struct {
	Description string                    `json:"description,omitempty"`
	Public      bool                      `json:"public,omitempty"`
	Files       map[GistFilename]GistFile `json:"files,omitempty"`
	HTMLURL     string                    `json:"html_url,omitempty"`
}

type GistFilename string

type GistFile struct {
	Content string `json:"content,omitempty"`
}

func apiCreate(httpClient *http.Client, hostname string, description string, public bool, files map[string]string) (*Gist, error) {
	gistFiles := map[GistFilename]GistFile{}

	for filename, content := range files {
		gistFiles[GistFilename(filename)] = GistFile{content}
	}

	path := "gists"
	body := &Gist{
		Description: description,
		Public:      public,
		Files:       gistFiles,
	}
	result := Gist{}

	requestByte, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	requestBody := bytes.NewReader(requestByte)

	apiClient := api.NewClientFromHTTP(httpClient)
	err = apiClient.REST(hostname, "POST", path, requestBody, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
