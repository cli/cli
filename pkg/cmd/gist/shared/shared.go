package shared

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/api"
)

// TODO make gist create use this file

type GistFile struct {
	Filename string `json:"filename"`
	Type     string `json:"type,omitempty"`
	Language string `json:"language,omitempty"`
	Content  string `json:"content"`
}

type Gist struct {
	ID          string               `json:"id,omitempty"`
	Description string               `json:"description"`
	Files       map[string]*GistFile `json:"files"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Public      bool                 `json:"public"`
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
