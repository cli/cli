package auth

import (
	"errors"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/go-gh/v2/pkg/auth"
)

var ErrUnsupportedHost = errors.New("An unsupported host was detected. Note that gh attestation does not currently support GHES")

func IsHostSupported() error {
	host, _ := auth.DefaultHost()

	// Note that this check is slightly redundant as Tenancy should not be considered Enterprise
	// but the ghinstance package has not been updated to reflect this yet.
	if ghinstance.IsEnterprise(host) && !ghinstance.IsTenancy(host) {
		return ErrUnsupportedHost
	}
	return nil
}
