package attachments

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func NewTestAssets(t *testing.T, names ...string) []UserAsset {
	t.Helper()
	t.Chdir(t.TempDir())

	argv := make([]string, 0, len(names)*2)
	for _, name := range names {
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
		require.NoError(t, os.WriteFile(name, []byte("the bytes"), 0o600))
		argv = append(argv, "--attach", "./"+name)
	}

	cmd := &cobra.Command{}
	attachFlag := AddFlag(cmd)
	require.NoError(t, cmd.Flags().Parse(argv))

	assets, err := attachFlag.UserAssets()
	require.NoError(t, err)
	return assets
}

func StubUpload(reg *httpmock.Registry, repositoryID int64, name string, status int, body string) {
	reg.Register(uploadMatcher(repositoryID, name), httpmock.StatusStringResponse(status, body))
}

func StubUploadToHost(t *testing.T, reg *httpmock.Registry, host string, repositoryID int64, name string, status int, body string) {
	reg.Register(uploadMatcher(repositoryID, name), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, host, req.URL.Host)
		return httpmock.StatusStringResponse(status, body)(req)
	})
}

func uploadMatcher(repositoryID int64, name string) httpmock.Matcher {
	return httpmock.QueryMatcher("POST", "user-attachments/assets", url.Values{
		"repository_id": []string{strconv.FormatInt(repositoryID, 10)},
		"name":          []string{name},
	})
}

type UploadStub struct {
	Name   string
	Status int
	Body   string
}
