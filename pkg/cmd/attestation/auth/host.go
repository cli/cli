package auth

import (
	"errors"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

var ErrUnsupportedHost = errors.New("An unsupported host was detected. Note that gh attestation does not currently support GHES")

func IsHostSupported(host string) error {
	if ghauth.IsEnterprise(host) {
		return ErrUnsupportedHost
	}
	return nil
}
