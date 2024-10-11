package auth

import (
	"errors"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

var ErrUnsupportedHost = errors.New("An unsupported host was detected. Note that gh attestation does not currently support GHES")

func IsHostSupported(host string) error {
	// Note that this check is slightly redundant as Tenancy should not be considered Enterprise
	// but the ghinstance package has not been updated to reflect this yet.
	if ghauth.IsEnterprise(host) && !ghauth.IsTenancy(host) {
		return ErrUnsupportedHost
	}
	return nil
}
