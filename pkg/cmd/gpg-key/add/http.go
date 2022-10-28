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

var scopesError = errors.New("insufficient OAuth scopes")

func gpgKeyUpload(httpClient *http.Client, hostname string, keyFile io.Reader) error {
	url := ghinstance.RESTPrefix(hostname) + "user/gpg_keys"

	keyBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"armored_public_key": string(keyBytes),
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
		return scopesError
	} else if resp.StatusCode > 299 {
		var httpError api.HTTPError
		err := api.HandleHTTPError(resp)
		if errors.As(err, &httpError) && isDuplicateError(&httpError) {
			return nil
		}
		if errors.As(err, &httpError) && isKeyInvalidError(&httpError) {
			if !isGpgKeyArmored(keyBytes) {
				return errors.New("it seems that the GPG key is not armored.\n" +
					"please try to find your GPG key ID using:\n" +
					"\tgpg --list-keys\n" +
					"and use command below to add it to your accont:\n" +
					"\tgpg --armor --export <GPG key ID> | gh gpg-key add -")
			}
		}
		return err
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func isDuplicateError(err *api.HTTPError) bool {
	return err.StatusCode == 422 && len(err.Errors) == 2 &&
		err.Errors[0].Field == "key_id" && err.Errors[0].Message == "key_id already exists"
}

func isKeyInvalidError(err *api.HTTPError) bool {
	return err.StatusCode == 422 && len(err.Errors) == 1 &&
		err.Errors[0].Field == "" && err.Errors[0].Message == "We got an error doing that."
}

func isGpgKeyArmored(keyBytes []byte) bool {
	buf := make([]byte, 36)
	copy(buf, keyBytes)
	return bytes.Equal(buf, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----"))
}
