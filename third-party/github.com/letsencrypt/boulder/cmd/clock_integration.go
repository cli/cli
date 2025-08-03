//go:build integration

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/jmhodges/clock"

	blog "github.com/letsencrypt/boulder/log"
)

// Clock functions similarly to clock.New(), but the returned value can be
// changed using the FAKECLOCK environment variable if the 'integration' build
// flag is set.
//
// The FAKECLOCK env var is in the time.UnixDate format, returned by `date -d`.
func Clock() clock.Clock {
	if tgt := os.Getenv("FAKECLOCK"); tgt != "" {
		targetTime, err := time.Parse(time.UnixDate, tgt)
		FailOnError(err, fmt.Sprintf("cmd.Clock: bad format for FAKECLOCK: %v\n", err))

		cl := clock.NewFake()
		cl.Set(targetTime)
		blog.Get().Debugf("Time was set to %v via FAKECLOCK", targetTime)
		return cl
	}

	return clock.New()
}
