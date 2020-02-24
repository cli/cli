package api

import (
	"bytes"
	"encoding/json"
	"time"

	"io/ioutil"
	"os"
)

// Gist represents a GitHub's gist.
type Gist struct {
	ID          string                    `json:"id,omitempty"`
	Description string                    `json:"description,omitempty"`
	Public      bool                      `json:"public,omitempty"`
	Files       map[GistFilename]GistFile `json:"files,omitempty"`
	Comments    int                       `json:"comments,omitempty"`
	HTMLURL     string                    `json:"html_url,omitempty"`
	GitPullURL  string                    `json:"git_pull_url,omitempty"`
	GitPushURL  string                    `json:"git_push_url,omitempty"`
	CreatedAt   time.Time                 `json:"created_at,omitempty"`
	UpdatedAt   time.Time                 `json:"updated_at,omitempty"`
	NodeID      string                    `json:"node_id,omitempty"`
}

// GistFilename represents filename on a gist.
type GistFilename string

// GistFile represents a file on a gist.
type GistFile struct {
	Size     int    `json:"size,omitempty"`
	Filename string `json:"filename,omitempty"`
	Language string `json:"language,omitempty"`
	Type     string `json:"type,omitempty"`
	RawURL   string `json:"raw_url,omitempty"`
	Content  string `json:"content,omitempty"`
}

// Create a gist for authenticated user.
//
// GitHub API docs: https://developer.github.com/v3/gists/#create-a-gist
func GistCreate(client *Client, description string, public bool, file string) (*Gist, error) {

	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	content := string(b)

	fileStat, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	fileName := GistFilename(fileStat.Name())

	gistFile := map[GistFilename]GistFile{}
	gistFile[fileName] = GistFile{Content: content}

	path := "gists"
	body := &Gist{
		Description: description,
		Public:      public,
		Files:       gistFile,
	}
	result := Gist{}

	requestByte, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	requestBody := bytes.NewReader(requestByte)

	err = client.REST("POST", path, requestBody, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
