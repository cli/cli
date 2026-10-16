package shared

import (
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeginAuthTelemetry_gating(t *testing.T) {
	t.Run("returns nil when the recorder is nil", func(t *testing.T) {
		assert.Nil(t, BeginAuthTelemetry(nil, "login", "github.com", true))
	})

	t.Run("returns nil for an enterprise host", func(t *testing.T) {
		recorder := &telemetry.InvocationRecorderSpy{}
		assert.Nil(t, BeginAuthTelemetry(recorder, "login", "ghe.example.com", true))
		assert.Empty(t, recorder.Events())
	})

	t.Run("records for github.com", func(t *testing.T) {
		recorder := &telemetry.InvocationRecorderSpy{}
		event := BeginAuthTelemetry(recorder, "login", "github.com", true)
		require.NotNil(t, event)

		assert.Equal(t, ghtelemetry.SAMPLE_ALL, recorder.LastSampleRate)
		events := recorder.Events()
		require.Len(t, events, 1)
		assert.Equal(t, "auth_invocation", events[0].Type)
		assert.Equal(t, "login", events[0].Dimensions["auth_command"])
		assert.Equal(t, "true", events[0].Dimensions["expiring_requested"])
		assert.Equal(t, "false", events[0].Dimensions["expiring_setup_cred_helper_cancelled"])
		assert.Equal(t, "false", events[0].Dimensions["auth_success"])
	})

	t.Run("records for a tenant host", func(t *testing.T) {
		recorder := &telemetry.InvocationRecorderSpy{}
		assert.NotNil(t, BeginAuthTelemetry(recorder, "refresh", "tenant.ghe.com", false))
		events := recorder.Events()
		require.Len(t, events, 1)
		assert.Equal(t, "refresh", events[0].Dimensions["auth_command"])
		assert.Equal(t, "false", events[0].Dimensions["expiring_requested"])
	})
}

func TestAuthTelemetryEvent_records(t *testing.T) {
	recorder := &telemetry.InvocationRecorderSpy{}
	event := BeginAuthTelemetry(recorder, "login", "github.com", false)

	event.RecordExpiringSetupCancelled(true)
	event.RecordHelperBefore("other")
	event.RecordHelperAfter("ours")
	event.RecordGitProtocol("ssh")
	event.RecordExpiringReceived(true)
	event.RecordStorage(true)
	event.RecordSuccess()

	events := recorder.Events()
	require.Len(t, events, 1)
	dims := events[0].Dimensions
	assert.Equal(t, "true", dims["expiring_setup_cred_helper_cancelled"])
	assert.Equal(t, "other", dims["git_credential_helper_before"])
	assert.Equal(t, "ours", dims["git_credential_helper_after"])
	assert.Equal(t, "ssh", dims["git_protocol"])
	assert.Equal(t, "true", dims["expiring_received"])
	assert.Equal(t, "plaintext", dims["storage_type"])
	assert.Equal(t, "true", dims["auth_success"])
}

func TestAuthTelemetryEvent_recordGitProtocolUnset(t *testing.T) {
	recorder := &telemetry.InvocationRecorderSpy{}
	event := BeginAuthTelemetry(recorder, "login", "github.com", false)
	event.RecordGitProtocol("")

	assert.Equal(t, "unset", recorder.Events()[0].Dimensions["git_protocol"])
}

func TestAuthTelemetryEvent_recordStorageKeyring(t *testing.T) {
	recorder := &telemetry.InvocationRecorderSpy{}
	event := BeginAuthTelemetry(recorder, "login", "github.com", false)
	event.RecordStorage(false)

	assert.Equal(t, "keyring", recorder.Events()[0].Dimensions["storage_type"])
}

func TestAuthTelemetryEvent_nilIsNoOp(t *testing.T) {
	var event *AuthTelemetryEvent

	assert.NotPanics(t, func() {
		event.RecordExpiringSetupCancelled(true)
		event.RecordHelperBefore("none")
		event.RecordHelperAfter("ours")
		event.RecordGitProtocol("https")
		event.RecordExpiringReceived(true)
		event.RecordStorage(true)
		event.RecordSuccess()
	})
}

func TestHelperCategory(t *testing.T) {
	tests := []struct {
		name   string
		helper gitcredentials.Helper
		want   string
	}{
		{name: "ours", helper: gitcredentials.Helper{Cmd: "!/path/to/gh auth git-credential"}, want: "ours"},
		{name: "other", helper: gitcredentials.Helper{Cmd: "osxkeychain"}, want: "other"},
		{name: "none", helper: gitcredentials.Helper{}, want: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, helperCategory(tt.helper))
		})
	}
}
