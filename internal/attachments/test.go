package attachments

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewTestAssets returns a UserAsset per name, resolved through --attach, which
// is the only way a command builds one.
//
// It writes each name into a temporary directory and changes the working
// directory to it for the rest of the test, so the names are relative paths
// that resolve.
func NewTestAssets(t *testing.T, names ...string) []UserAsset {
	t.Helper()

	t.Chdir(t.TempDir())

	argv := make([]string, 0, len(names)*2)
	for _, name := range names {
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

// NewTestInvocationTelemetry returns telemetry started with the given
// attachment count.
func NewTestInvocationTelemetry(t *testing.T, recorder ghtelemetry.CommandRecorder, attachCount int) *InvocationTelemetry {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	attachFlag := AddFlag(cmd)
	for i := range attachCount {
		require.NoError(t, cmd.Flags().Set(flagName, "attachment-"+strconv.Itoa(i)+".png"))
	}
	invocationTelemetry := NewInvocationTelemetry(attachFlag, recorder)
	invocationTelemetry.start("gh test")
	return invocationTelemetry
}

// AssertTestTelemetryEvents verifies the completed invocation event shape.
func AssertTestTelemetryEvents(t *testing.T, events []ghtelemetry.Event, attachCount int, result UploadResult) {
	t.Helper()

	require.Equal(t, []ghtelemetry.Event{
		{
			Type:       "attachment_invocation",
			Dimensions: ghtelemetry.Dimensions{"command": "gh test"},
			Measures: ghtelemetry.Measures{
				"attach_count":      int64(attachCount),
				"append_ops_count":  int64(result.AppendOperations),
				"replace_ops_count": int64(result.ReplaceOperations),
			},
		},
	}, events)
}

// StubUpload registers one upload of name against repositoryID, answering with
// status and body.
//
// It matches on the query the upload carries, so a request that names the wrong
// repository or the wrong file matches nothing and the test fails.
func StubUpload(reg *httpmock.Registry, repositoryID int64, name string, status int, body string) {
	reg.Register(uploadMatcher(repositoryID, name), httpmock.StatusStringResponse(status, body))
}

// StubUploadToHost is StubUpload plus the host the request reached. The
// matchers compare the path only, so the host is asserted in the responder or
// not at all.
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

// UploadStub is one stubbed reply from the asset upload endpoint, named for the
// file it answers for. StubUpload registers one.
type UploadStub struct {
	Name   string
	Status int
	Body   string
}
