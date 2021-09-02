package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
)

var scopesError = errors.New("insufficient OAuth scopes")

type sshKey struct {
	Key       string
	Title     string
	CreatedAt time.Time `json:"created_at"`
}

func userKeys(httpClient *http.Client, host, userHandle string) ([]sshKey, error) {
	resource := "user/keys"
	if userHandle != "" {
		resource = fmt.Sprintf("users/%s/keys", userHandle)
	}
	url := fmt.Sprintf("%s%s?per_page=%d", ghinstance.RESTPrefix(host), resource, 100)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, scopesError
	} else if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var keys []sshKey
	err = json.Unmarshal(b, &keys)
	if err != nil {
		return nil, err
	}

	return keys, nil
}
