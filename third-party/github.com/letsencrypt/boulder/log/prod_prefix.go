//go:build !integration

package log

import (
	"fmt"
	"os"
	"strings"

	"github.com/letsencrypt/boulder/core"
)

// getPrefix returns the prefix and clkFormat that should be used by the
// stdout logger.
func getPrefix() (string, string) {
	shortHostname := "unknown"
	datacenter := "unknown"
	hostname, err := os.Hostname()
	if err == nil {
		splits := strings.SplitN(hostname, ".", 3)
		shortHostname = splits[0]
		if len(splits) > 1 {
			datacenter = splits[1]
		}
	}

	prefix := fmt.Sprintf("%s %s %s[%d]: ", shortHostname, datacenter, core.Command(), os.Getpid())
	clkFormat := "2006-01-02T15:04:05.000000+00:00Z"

	return prefix, clkFormat
}
