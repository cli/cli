package auth

import (
	"errors"
	"strings"

	"github.com/cli/go-gh/v2/pkg/auth"
)

var ErrUnsupportedHost = errors.New("An unsupported host was detected. Note that gh attestation does not currently support GHES")

const (
	github    = "github.com"
	localhost = "github.localhost"
	// tenancyHost is the domain name of a tenancy GitHub instance
	tenancyHost = "ghe.com"
)

func isEnterprise(host string) bool {
	return host != github && host != localhost && !isTenancy(host)
}

func isTenancy(host string) bool {
	return strings.HasSuffix(host, "."+tenancyHost)
}

func IsHostSupported() error {
	host, _ := auth.DefaultHost()

	if isEnterprise(host) {
		return ErrUnsupportedHost
	}
	return nil
}
