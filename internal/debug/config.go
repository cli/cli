// Package debug provides build-gated configuration for internal debugging.
package debug

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	o "github.com/cli/cli/v2/pkg/option"
)

// DebugConfig holds explicit internal debug overrides.
// A None field means no override; a Some field applies its value even when that value is zero or false.
type DebugConfig struct {
	// AccessTokenExpiresIn overrides the access token lifetime using GH_INTERNAL_AT_EXPIRES_IN.
	// The environment value is an integer number of seconds; None means no override.
	AccessTokenExpiresIn o.Option[time.Duration]

	// RefreshTokenExpiresIn overrides the refresh token lifetime using GH_INTERNAL_RT_EXPIRES_IN.
	// The environment value is an integer number of seconds; None means no override.
	RefreshTokenExpiresIn o.Option[time.Duration]

	// VerboseTokenRefresh overrides full refresh traffic logging using GH_INTERNAL_DEBUG_REFRESH_TOKEN.
	// The environment value must be accepted by strconv.ParseBool; None means no override.
	VerboseTokenRefresh o.Option[bool]
}

// configFromEnv builds a DebugConfig from the environment, panicking on invalid values. It is not build-gated so
// its parsing is tested in regular builds; only debug builds call it from Config.
func configFromEnv() DebugConfig {
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
