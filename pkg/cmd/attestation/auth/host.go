package auth

import (
	"errors"

	"github.com/cli/go-gh/v2/pkg/auth"
)

var ErrUnsupportedHost = errors.New("The GH_HOST environment variable is set to a custom GitHub host. gh attestation does not currently support custom GitHub Enterprise hosts")

func IsHostSupported() error {
	host, _ := auth.DefaultHost()
	if host != "github.com" {
		return ErrUnsupportedHost
	}
	return nil
}
