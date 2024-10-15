package authflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getCallbackURI(t *testing.T) {
	tests := []struct {
		name      string
		oauthHost string
		want      string
	}{
		{
			name:      "dotcom",
			oauthHost: "github.com",
			want:      "http://127.0.0.1/callback",
		},
		{
			name:      "ghes",
			oauthHost: "my.server.com",
			want:      "http://localhost/",
		},
		{
			name:      "ghec data residency (ghe.com)",
			oauthHost: "stampname.ghe.com",
			want:      "http://127.0.0.1/callback",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getCallbackURI(tt.oauthHost))
		})
	}
}
