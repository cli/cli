package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
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

	keys, err := UserKeys(&http.Client{Transport: reg}, "github.com", "")

	assert.Nil(t, keys)
	var httpErr api.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
}
