package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCI(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "no CI env vars", env: map[string]string{}, want: false},
		{name: "CI set", env: map[string]string{"CI": "true"}, want: true},
		{name: "BUILD_NUMBER set", env: map[string]string{"BUILD_NUMBER": "42"}, want: true},
		{name: "RUN_ID set", env: map[string]string{"RUN_ID": "abc"}, want: true},
		{name: "CI empty string", env: map[string]string{"CI": ""}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CI", "")
			t.Setenv("BUILD_NUMBER", "")
			t.Setenv("RUN_ID", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			assert.Equal(t, tt.want, IsCI())
		})
	}
}

func TestIsGitHubActions(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		want  bool
	}{
		{name: "unset", set: false, want: false},
		{name: "true", value: "true", set: true, want: true},
		{name: "false", value: "false", set: true, want: false},
		{name: "empty", value: "", set: true, want: false},
		{name: "other value", value: "yes", set: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", "")
			if tt.set {
				t.Setenv("GITHUB_ACTIONS", tt.value)
			}
			assert.Equal(t, tt.want, IsGitHubActions())
		})
	}
}
