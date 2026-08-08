package add

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

var errScopesMissing = errors.New("insufficient OAuth scopes")
var errDuplicateKey = errors.New("key already exists")
var errWrongFormat = errors.New("key in wrong format")

func gpgKeyUpload(httpClient *http.Client, hostname string, keyFile io.Reader, title string) error {
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

	path, err := safeurl.JoinPath("user", "gpg_keys")
	if err != nil {
		return err
	}

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	apiClient := api.NewClientFromHTTP(httpClient)
	err = apiClient.REST(hostname, "POST", path.String(), bytes.NewBuffer(payloadBytes), nil)
	if err != nil {
		if httpError, ok := errors.AsType[api.HTTPError](err); ok {
			if httpError.StatusCode == 404 {
				return errScopesMissing
			}
			for _, e := range httpError.Errors {
				if httpError.StatusCode == 422 && e.Field == "key_id" && e.Message == "key_id already exists" {
					return errDuplicateKey
				}
			}
			if httpError.StatusCode == 422 && !isGpgKeyArmored(keyBytes) {
				return errWrongFormat
			}
		}
		return err
	}

	return nil
}

func isGpgKeyArmored(keyBytes []byte) bool {
	return bytes.Contains(keyBytes, []byte("-----BEGIN "))
}
