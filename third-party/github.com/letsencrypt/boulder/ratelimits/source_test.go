package ratelimits

import (
	"testing"

	"github.com/jmhodges/clock"
)

func newInmemTestLimiter(t *testing.T, clk clock.FakeClock) *Limiter {
	return newTestLimiter(t, newInmem(), clk)
}
