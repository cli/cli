package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
)

func deleteSSHKey(httpClient *http.Client, host string, keyID string) error {
	url := fmt.Sprintf("%suser/keys/%s", ghinstance.RESTPrefix(host), keyID)
	req, err := http.NewRequest("DELETE", url, nil)
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

	return nil
}
