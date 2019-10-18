package context

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type viewer struct {
	Login string
}
type responseData struct {
	Data struct {
		Viewer *viewer
	}
}

// TODO: figure out how to enable using the "api" package here
//
// Right now "api" is coupled to "context", so we can't import "api" from here.
func getViewer(token string) (user *viewer, err error) {
	url := "https://api.github.com/graphql"
	query := `{ viewer { login } }`

	reqBody, err := json.Marshal(map[string]interface{}{"query": query})
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	data := responseData{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return
	}

	user = data.Data.Viewer
	return
}
