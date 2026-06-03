package ghenv

import (
	"os"
	"slices"
)

func ReadOnly() bool {
	value, ok := os.LookupEnv("GH_READ_ONLY")
	if !ok {
		return false
	}
	return !slices.Contains([]string{"false", "0", "no", ""}, value)
}
