package shared

import (
	"context"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserKeysHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "user/keys"),
		httpmock.StatusStringResponse(http.StatusNotFound, `{"message":"Not Found"}`),
	)

	client, err := httpmock.RESTClientFunc(reg)("github.com")
	require.NoError(t, err)
	keys, err := UserKeys(context.Background(), client, "")

	assert.Nil(t, keys)
	var httpErr *githubrest.ErrorResponse
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}
