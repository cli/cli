package githubrest_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

// The sanitizer has to live on the Client rather than on the *http.Client it is
// given, because callers assemble transports in several places and a missing
// wrapper would send terminal escape sequences straight to a user's screen.
func TestClientSanitizesControlCharactersInJSONResponses(t *testing.T) {
	t.Run("JSON bodies are sanitized", func(t *testing.T) {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", "gists/1234"),
			httpmock.JSONResponse(map[string]string{"description": "danger\x1b[31m"}),
		)

		client, err := httpmock.RESTClientFunc(reg)("github.com")
		require.NoError(t, err)

		req, err := client.NewRequest(context.Background(), http.MethodGet, "gists/1234", nil)
		require.NoError(t, err)

		var got struct {
			Description string `json:"description"`
		}
		_, err = client.Do(req, &got)
		require.NoError(t, err)
		require.Equal(t, "danger^[[31m", got.Description)
	})

	t.Run("non-JSON bodies are left alone", func(t *testing.T) {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", "download"),
			// Invalid UTF-8, which the sanitizer would reject outright. Asset and
			// log downloads look like this.
			func(*http.Request) (*http.Response, error) {
				return httpmock.StringResponse("\xff\xfe\x00binary")(nil)
			},
		)

		client, err := httpmock.RESTClientFunc(reg)("github.com")
		require.NoError(t, err)

		req, err := client.NewRequest(context.Background(), http.MethodGet, "download", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "\xff\xfe\x00binary", string(body))
	})
}
