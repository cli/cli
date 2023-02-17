package add

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
)

var errScopesMissing = errors.New("insufficient OAuth scopes")
var errDuplicateKey = errors.New("key already exists")
var errWrongFormat = errors.New("key in wrong format")

func gpgKeyUpload(httpClient *http.Client, hostname string, keyFile io.Reader, title string) error {
	url := ghinstance.RESTPrefix(hostname) + "user/gpg_keys"

	keyBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"armored_public_key": string(keyBytes),
	}
	if title != "" {
		payload["name"] = title
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return errScopesMissing
	} else if resp.StatusCode > 299 {
		err := api.HandleHTTPError(resp)
		var httpError api.HTTPError
		if errors.As(err, &httpError) {
			for _, e := range httpError.Errors {
				if resp.StatusCode == 422 && e.Field == "key_id" && e.Message == "key_id already exists" {
					return errDuplicateKey
				}
			}
		}
		if resp.StatusCode == 422 && !isGpgKeyArmored(keyBytes) {
			return errWrongFormat
		}
		return err
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func isGpgKeyArmored(keyBytes []byte) bool {
	return bytes.Contains(keyBytes, []byte("-----BEGIN "))
}
