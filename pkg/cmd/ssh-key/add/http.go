package add

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
)

// SSHKeyUpload uploads the provided SSH key. Returns true if the key was uploaded, false if it was not.
// reqOpts exists for the login flow, which uploads a key with a token that is
// not yet stored in config and so must be supplied per request.
func SSHKeyUpload(ctx context.Context, client *githubrest.Client, keyFile io.Reader, title string, reqOpts ...githubrest.RequestOption) (bool, error) {
	u, err := safeurl.JoinPath("user", "keys")
	if err != nil {
		return false, err
	}

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

	keys, err := shared.UserKeys(ctx, client, "", reqOpts...)
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

	err = keyUpload(ctx, client, u, payload, reqOpts...)

	if err != nil {
		return false, err
	}

	return true, nil
}

// SSHSigningKeyUpload uploads the provided SSH Signing key. Returns true if the key was uploaded, false if it was not.
func SSHSigningKeyUpload(ctx context.Context, client *githubrest.Client, keyFile io.Reader, title string, reqOpts ...githubrest.RequestOption) (bool, error) {
	u, err := safeurl.JoinPath("user", "ssh_signing_keys")
	if err != nil {
		return false, err
	}

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

	keys, err := shared.UserSigningKeys(ctx, client, "", reqOpts...)
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

	err = keyUpload(ctx, client, u, payload, reqOpts...)

	if err != nil {
		return false, err
	}

	return true, nil
}

func keyUpload(ctx context.Context, client *githubrest.Client, u safeurl.SafeURL, payload map[string]string, reqOpts ...githubrest.RequestOption) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := client.NewRequest(ctx, http.MethodPost, u.String(), bytes.NewBuffer(payloadBytes), reqOpts...)
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}
