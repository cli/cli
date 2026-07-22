package env_test

import (
	"testing"

	"github.com/cli/cli/v2/internal/env"
	"github.com/stretchr/testify/assert"
)

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		setEnv bool
		want   bool
	}{
		{name: "unset env var", setEnv: false, want: false},
		{name: "empty string", value: "", setEnv: true, want: false},
		{name: "zero", value: "0", setEnv: true, want: false},
		{name: "false", value: "false", setEnv: true, want: false},
		{name: "FALSE", value: "FALSE", setEnv: true, want: false},
		{name: "False", value: "False", setEnv: true, want: false},
		{name: "no", value: "no", setEnv: true, want: false},
		{name: "NO", value: "NO", setEnv: true, want: false},
		{name: "disabled", value: "disabled", setEnv: true, want: false},
		{name: "off", value: "off", setEnv: true, want: false},
		{name: "whitespace around falsey value", value: "  false  ", setEnv: true, want: false},
		{name: "1", value: "1", setEnv: true, want: true},
		{name: "true", value: "true", setEnv: true, want: true},
		{name: "TRUE", value: "TRUE", setEnv: true, want: true},
		{name: "yes", value: "yes", setEnv: true, want: true},
		{name: "enabled", value: "enabled", setEnv: true, want: true},
		{name: "on", value: "on", setEnv: true, want: true},
		{name: "arbitrary string", value: "banana", setEnv: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const envKey = "GH_TEST_IS_TRUTHY"
			if tt.setEnv {
				t.Setenv(envKey, tt.value)
			}
			assert.Equal(t, tt.want, env.IsTruthy(envKey))
		})
	}
}
