package add

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

var errScopesMissing = errors.New("insufficient OAuth scopes")
var errDuplicateKey = errors.New("key already exists")
var errWrongFormat = errors.New("key in wrong format")

func gpgKeyUpload(ctx context.Context, client *githubrest.Client, keyFile io.Reader, title string) error {
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

	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	if err != nil {
		if httpError, ok := errors.AsType[*githubrest.ErrorResponse](err); ok {
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
