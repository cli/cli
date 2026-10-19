//go:build ghdebug

package debug

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	o "github.com/cli/cli/v2/pkg/option"
)

// Enabled reports whether gh was built with the ghdebug build tag.
const Enabled = true

// Config reads internal debug overrides from the environment.
// Without the ghdebug build tag it ignores the environment and returns a DebugConfig with no overrides set.
// In debug builds, invalid environment values cause a panic.
func Config() DebugConfig {
	verboseTokenRefresh := o.None[bool]()
	if raw, ok := os.LookupEnv("GH_INTERNAL_DEBUG_REFRESH_TOKEN"); ok {
		verbose, err := strconv.ParseBool(raw)
		if err != nil {
			panic(fmt.Errorf("GH_INTERNAL_DEBUG_REFRESH_TOKEN: %w", err))
		}
		verboseTokenRefresh = o.Some(verbose)
	}
	return DebugConfig{
		AccessTokenExpiresIn:  lifetimeFromEnv("GH_INTERNAL_AT_EXPIRES_IN"),
		RefreshTokenExpiresIn: lifetimeFromEnv("GH_INTERNAL_RT_EXPIRES_IN"),
		VerboseTokenRefresh:   verboseTokenRefresh,
	}
}

func lifetimeFromEnv(name string) o.Option[time.Duration] {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return o.None[time.Duration]()
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		panic(fmt.Errorf("%s: %w", name, err))
	}
	if seconds > math.MaxInt64/int64(time.Second) || seconds < math.MinInt64/int64(time.Second) {
		panic(fmt.Errorf("%s: lifetime in seconds overflows time.Duration", name))
	}
	return o.Some(time.Duration(seconds) * time.Second)
}
