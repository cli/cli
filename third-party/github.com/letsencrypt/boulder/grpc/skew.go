//go:build !integration

package grpc

import "time"

// tooSkewed returns true if the absolute value of the input duration is more
// than ten minutes. We break this out into a separate function so that it can
// be disabled in the integration tests, which make extensive use of fake
// clocks.
func tooSkewed(skew time.Duration) bool {
	return skew > 10*time.Minute || skew < -10*time.Minute
}
