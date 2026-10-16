package shared

import (
	"fmt"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

// AuthTelemetryEvent tracks the auth-state facts of a single login or refresh invocation in a pending telemetry event.
// [ghtelemetry.Service.Finish] owns completion; later updates are ignored. All methods are nil-tolerant so callers can
// hold a nil event when telemetry is disabled or the host is not eligible.
type AuthTelemetryEvent struct {
	pendingEvent ghtelemetry.PendingEvent
}

// BeginAuthTelemetry starts an auth-state event at full sampling for a login or refresh invocation. It returns nil when
// recorder is nil or when hostname is a GitHub Enterprise Server host, since enterprise usage is never reported. command
// is "login" or "refresh" and expiringRequested is the effective intent to request an expiring, refreshable token.
func BeginAuthTelemetry(recorder ghtelemetry.InvocationRecorder, command, hostname string, expiringRequested bool) *AuthTelemetryEvent {
	if recorder == nil || ghauth.IsEnterprise(hostname) {
		return nil
	}

	recorder.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	pendingEvent := recorder.Begin(ghtelemetry.Event{
		Type: "auth_invocation",
		Dimensions: ghtelemetry.Dimensions{
			"auth_command":                         command,
			"expiring_requested":                   fmt.Sprintf("%v", expiringRequested),
			"expiring_setup_cred_helper_cancelled": "false",
			"auth_success":                         "false",
		},
	})

	return &AuthTelemetryEvent{pendingEvent: pendingEvent}
}

// RecordExpiringSetupCancelled records whether the user cancelled the sign-in at the expiring credential-helper prompt.
// That prompt, and therefore a cancellation, is only reachable on an expiring login.
func (e *AuthTelemetryEvent) RecordExpiringSetupCancelled(cancelled bool) {
	if e == nil {
		return
	}
	e.pendingEvent.UpsertDimensions(ghtelemetry.Dimensions{"expiring_setup_cred_helper_cancelled": fmt.Sprintf("%v", cancelled)})
}

// RecordGitProtocol captures the git operations protocol selected for the host, as "https" or "ssh", or "unset" when no
// protocol was explicitly selected (a non-interactive login without the git-protocol flag).
func (e *AuthTelemetryEvent) RecordGitProtocol(protocol string) {
	if e == nil {
		return
	}
	if protocol == "" {
		protocol = "unset"
	}
	e.pendingEvent.UpsertDimensions(ghtelemetry.Dimensions{"git_protocol": protocol})
}

// RecordHelperBefore captures the git credential helper configured before login reconfigured it, as one of "ours",
// "other", "none", or "error" when the helper could not be read.
func (e *AuthTelemetryEvent) RecordHelperBefore(category string) {
	if e == nil {
		return
	}
	e.pendingEvent.UpsertDimensions(ghtelemetry.Dimensions{"git_credential_helper_before": category})
}

// RecordHelperAfter captures the git credential helper configured after login completed, as one of "ours", "other",
// "none", or "error" when the helper could not be read.
func (e *AuthTelemetryEvent) RecordHelperAfter(category string) {
	if e == nil {
		return
	}
	e.pendingEvent.UpsertDimensions(ghtelemetry.Dimensions{"git_credential_helper_after": category})
}

// RecordExpiringReceived captures whether the server issued an expiring, refreshable token.
func (e *AuthTelemetryEvent) RecordExpiringReceived(received bool) {
	if e == nil {
		return
	}
	e.pendingEvent.UpsertDimensions(ghtelemetry.Dimensions{"expiring_received": fmt.Sprintf("%v", received)})
}

// RecordStorage captures where the credential was stored: keyring when a secure store was used, plaintext otherwise.
func (e *AuthTelemetryEvent) RecordStorage(insecureStorageUsed bool) {
	if e == nil {
		return
	}
	storage := "keyring"
	if insecureStorageUsed {
		storage = "plaintext"
	}
	e.pendingEvent.UpsertDimensions(ghtelemetry.Dimensions{"storage_type": storage})
}

// RecordSuccess marks the invocation as having stored a credential successfully.
func (e *AuthTelemetryEvent) RecordSuccess() {
	if e == nil {
		return
	}
	e.pendingEvent.UpsertDimensions(ghtelemetry.Dimensions{"auth_success": "true"})
}
