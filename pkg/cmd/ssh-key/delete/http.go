package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
)

type sshKey struct {
	Title string
}

func deleteSSHKey(httpClient *http.Client, host string, keyID string) error {
	path := fmt.Sprintf("user/keys/%s", keyID)
	return api.NewClientFromHTTP(httpClient).REST(host, "DELETE", path, nil, nil)
}

func getSSHKey(httpClient *http.Client, host string, keyID string) (*sshKey, error) {
	path := fmt.Sprintf("user/keys/%s", keyID)
	var key sshKey
	if err := api.NewClientFromHTTP(httpClient).REST(host, "GET", path, nil, &key); err != nil {
		return nil, err
	}
	return &key, nil
}
