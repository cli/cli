package config

import (
	"fmt"
	"strings"
)

// ParseAccount splits a "user@host" account identifier into its user and host
// components. Returns an error with an actionable message if the format is invalid.
func ParseAccount(account string) (user, host string, err error) {
	idx := strings.LastIndex(account, "@")
	if idx < 1 || idx >= len(account)-1 {
		return "", "", fmt.Errorf(`invalid account format %q. Expected "user@host" (e.g. "monalisa@github.com")`, account)
	}
	return account[:idx], account[idx+1:], nil
}
