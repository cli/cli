package add

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
)

// Uploads the provided SSH key. Returns true if the key was uploaded, false if it was not.
func SSHKeyUpload(httpClient *http.Client, hostname string, keyFile io.Reader, title string) (bool, error) {
	url := ghinstance.RESTPrefix(hostname) + "user/keys"

	keyBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return false, err
	}

	fullUserKey := string(keyBytes)
	splitKey := strings.Fields(fullUserKey)
	if len(splitKey) < 2 {
		return false, errors.New("provided key is not in a valid format")
	}

	keyToCompare := splitKey[0] + " " + splitKey[1]

	keys, err := shared.UserKeys(httpClient, hostname, "")
	if err != nil {
		return false, err
	}

	for _, k := range keys {
		if k.Key == keyToCompare {
			return false, nil
		}
	}

	payload := map[string]string{
		"title": title,
		"key":   fullUserKey,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return false, api.HandleHTTPError(resp)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return false, err
	}

	return true, nil
}
