package add

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func SSHKeyUpload(httpClient *http.Client, hostname string, keyFile io.Reader, title string, printSuccessMsgs bool, ios *iostreams.IOStreams) error {
	url := ghinstance.RESTPrefix(hostname) + "user/keys"
	cs := ios.ColorScheme()

	keyBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return err
	}

	fullUserKey := string(keyBytes)
	splitKey := strings.Fields(fullUserKey)
	if len(splitKey) < 2 {
		fmt.Fprintf(ios.ErrOut, "%s Error: provided key is not in a valid format\n", cs.FailureIcon())
		return cmdutil.SilentError
	}

	keyToCompare := splitKey[0] + " " + splitKey[1]

	keys, err := shared.UserKeys(httpClient, hostname, "")
	if err != nil {
		return err
	}

	for _, k := range keys {
		if k.Key == keyToCompare {
			if printSuccessMsgs && ios.IsStderrTTY() {
				fmt.Fprintf(ios.ErrOut, "%s Public key already exists on your account\n", cs.SuccessIcon())
			}
			return nil
		}
	}

	payload := map[string]string{
		"title": title,
		"key":   fullUserKey,
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

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}

	if printSuccessMsgs && ios.IsStderrTTY() {
		fmt.Fprintf(ios.ErrOut, "%s Public key added to your account\n", cs.SuccessIcon())
	}
	return nil
}
