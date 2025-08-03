//go:build integration

package log

// getPrefix returns the prefix and clkFormat that should be used by the
// stdout logger.
func getPrefix() (string, string) {
	return "", "15:04:05.000000"
}
