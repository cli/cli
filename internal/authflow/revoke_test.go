package authflow

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

func TestRevokeToken(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST(http.MethodDelete, "applications/178c6fc778ccc68e1d6a/token"),
		httpmock.RESTPayload(http.StatusNoContent, "", func(payload map[string]any) {
			require.Equal(t, "gho_test-token", payload["access_token"])
		}),
	)

	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	require.NoError(t, RevokeToken(httpClient, "github.com", "gho_test-token"))

	request := reg.Requests[0]
	clientID, clientSecret, ok := request.BasicAuth()
	require.True(t, ok)
	require.Equal(t, oauthClientID, clientID)
	require.Equal(t, oauthClientSecret, clientSecret)
	require.Equal(t, "application/vnd.github+json", request.Header.Get("Accept"))
	require.Equal(t, "application/json", request.Header.Get("Content-Type"))
}

func TestRevokeTokenNotFoundIsIdempotent(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST(http.MethodDelete, "applications/178c6fc778ccc68e1d6a/token"),
		httpmock.StatusJSONResponse(http.StatusNotFound, map[string]string{"message": "Not Found"}),
	)

	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	require.NoError(t, RevokeToken(httpClient, "github.com", "gho_test-token"))
}

func TestRevokeTokenError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST(http.MethodDelete, "applications/178c6fc778ccc68e1d6a/token"),
		httpmock.StatusJSONResponse(http.StatusUnprocessableEntity, map[string]string{"message": "Validation Failed"}),
	)

	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	err := RevokeToken(httpClient, "github.com", "gho_test-token")
	require.EqualError(t, err, "HTTP 422: Validation Failed (https://api.github.com/applications/178c6fc778ccc68e1d6a/token)")
}
