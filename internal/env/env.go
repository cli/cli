package env

import (
	"os"
	"slices"
	"strings"
)

var falseyValues = []string{"", "0", "false", "no", "disabled", "off"}

func IsTruthy(name string) bool {
	envVal := strings.TrimSpace(strings.ToLower(os.Getenv(name)))

	if slices.Contains(falseyValues, envVal) {
		return false
	}

	return true
}
