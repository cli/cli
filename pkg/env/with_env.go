package env

import "os"

// WithEnv changes environment variables and returns a function to restore them to their original values.
func WithEnv(vars map[string]string) func() {
	originalValues := map[string]*string{}
	for name, value := range vars {
		if oldValue, ok := os.LookupEnv(name); ok {
			originalValues[name] = &oldValue
		} else {
			originalValues[name] = nil
		}
		os.Setenv(name, value)
	}

	return func() {
		for name, oldValue := range originalValues {
			if oldValue == nil {
				os.Unsetenv(name)
			} else {
				os.Setenv(name, *oldValue)
			}
		}
	}
}
