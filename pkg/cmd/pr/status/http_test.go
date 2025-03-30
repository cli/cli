package status

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_getCurrentUsername(t *testing.T) {
	tests := []struct {
		name            string
		username        string
		hostname        string
		serverUsername  string
		currentUsername string
	}{
		{
			name:            "dotcom",
			username:        "@me",
			hostname:        "github.com",
			currentUsername: "@me",
		},
		{
			name:            "ghec data residency (ghe.com)",
			username:        "@me",
			hostname:        "stampname.ghe.com",
			currentUsername: "@me",
		},
		{
			name:            "ghes",
			username:        "@me",
			hostname:        "my.server.com",
			serverUsername:  "@serverUserName",
			currentUsername: "@serverUserName",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientStub := &http.Client{}
			if tt.serverUsername != "" {
				reg := &httpmock.Registry{}
				defer reg.Verify(t)

				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"`+tt.serverUsername+`"}}}`),
				)

				clientStub.Transport = reg
			}

			apiClientStub := api.NewClientFromHTTP(clientStub)

			currentUsername, err := getCurrentUsername(tt.username, tt.hostname, apiClientStub)
			assert.NoError(t, err)
			assert.Equal(t, tt.currentUsername, currentUsername)
		})
	}
}
