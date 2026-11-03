//go:build !ghdebug

package debug

// Enabled reports whether gh was built with the ghdebug build tag.
const Enabled = false

// Config reads internal debug overrides from the environment.
// Without the ghdebug build tag it ignores the environment and returns a DebugConfig with no overrides set.
// In debug builds, invalid environment values cause a panic.
func Config() DebugConfig {
	return DebugConfig{}
}
