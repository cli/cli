package shared

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHelperConfig struct {
	helper gitcredentials.Helper
	err    error
}

func (s stubHelperConfig) ConfigureOurs(string) error { return nil }

func (s stubHelperConfig) ConfiguredHelper(string) (gitcredentials.Helper, error) {
	return s.helper, s.err
}

func TestRecordAuthInvocationTelemetry(t *testing.T) {
	tests := []struct {
		name           string
		helper         gitcredentials.Helper
		preferExpiring string
		wantState      string
	}{
		{
			name:           "gh is the configured helper",
			helper:         gitcredentials.Helper{Cmd: "!/usr/bin/gh auth git-credential"},
			preferExpiring: "enabled",
			wantState:      "ours",
		},
		{
			name:           "another helper is configured",
			helper:         gitcredentials.Helper{Cmd: "osxkeychain"},
			preferExpiring: "disabled",
			wantState:      "other",
		},
		{
			name:           "no helper is configured",
			helper:         gitcredentials.Helper{},
			preferExpiring: "disabled",
			wantState:      "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &telemetry.InvocationRecorderSpy{}

			RecordAuthInvocationTelemetry(recorder, "login", "github.com", stubHelperConfig{helper: tt.helper}, tt.preferExpiring)

			events := recorder.Events()
			require.Len(t, events, 1)
			assert.Equal(t, "auth_invocation", events[0].Type)
			assert.Equal(t, ghtelemetry.Dimensions{
				"auth_command":                "login",
				"git_credential_helper":       tt.wantState,
				"prefer_expiring_credentials": tt.preferExpiring,
			}, events[0].Dimensions)
			assert.Equal(t, ghtelemetry.SAMPLE_ALL, recorder.LastSampleRate)
		})
	}

	t.Run("does not record when the helper cannot be resolved", func(t *testing.T) {
		recorder := &telemetry.InvocationRecorderSpy{}

		RecordAuthInvocationTelemetry(recorder, "refresh", "github.com", stubHelperConfig{err: errors.New("git not installed")}, "disabled")

		assert.Empty(t, recorder.Events())
		assert.Equal(t, 0, recorder.LastSampleRate)
	})

	t.Run("is a no-op when the recorder is nil", func(t *testing.T) {
		assert.NotPanics(t, func() {
			RecordAuthInvocationTelemetry(nil, "login", "github.com", stubHelperConfig{}, "disabled")
		})
	})

	t.Run("is a no-op when the helper config is nil", func(t *testing.T) {
		recorder := &telemetry.InvocationRecorderSpy{}

		RecordAuthInvocationTelemetry(recorder, "login", "github.com", nil, "disabled")

		assert.Empty(t, recorder.Events())
	})
}
