//go:build integration

package grpc

import "time"

// tooSkewed always returns false, but is only built when the integration build
// flag is set. We use this to replace the real tooSkewed function in the
// integration tests, which make extensive use of fake clocks.
func tooSkewed(_ time.Duration) bool {
	return false
}
