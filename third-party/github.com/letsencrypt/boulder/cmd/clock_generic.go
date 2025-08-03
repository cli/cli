//go:build !integration

package cmd

import "github.com/jmhodges/clock"

// Clock functions similarly to clock.New(), but the returned value can be
// changed using the FAKECLOCK environment variable if the 'integration' build
// flag is set.
//
// This function returns the default Clock.
func Clock() clock.Clock {
	return clock.New()
}
