package shared

import "github.com/cli/cli/v2/internal/gh/ghtelemetry"

// Git credential helper states reported in the git_credential_helper dimension of the
// auth_invocation telemetry event.
const (
	gitCredentialHelperOurs  = "ours"  // gh is configured as the credential helper
	gitCredentialHelperOther = "other" // a non-gh credential helper is configured
	gitCredentialHelperNone  = "none"  // no credential helper is configured
)

// RecordAuthInvocationTelemetry records a best-effort event describing the git credential configuration observed
// when auth login or auth refresh runs: whether gh, another helper, or no helper is configured as the git credential
// helper for hostname, and the prefer_expiring_credentials setting in effect. authCommand identifies the invoking
// command (for example "login" or "refresh").
//
// It never fails the command: a nil recorder or helperConfig, or an error resolving the configured helper, results in
// no event being recorded. Recording sets full sampling so these infrequent auth events are retained.
func RecordAuthInvocationTelemetry(recorder ghtelemetry.InvocationRecorder, authCommand, hostname string, helperConfig HelperConfig, preferExpiringCredentials string) {
	if recorder == nil || helperConfig == nil {
		return
	}

	helper, err := helperConfig.ConfiguredHelper(hostname)
	if err != nil {
		return
	}

	state := gitCredentialHelperNone
	if helper.IsConfigured() {
		if helper.IsOurs() {
			state = gitCredentialHelperOurs
		} else {
			state = gitCredentialHelperOther
		}
	}

	recorder.SetSampleRate(ghtelemetry.SAMPLE_ALL)
	recorder.Record(ghtelemetry.Event{
		Type: "auth_invocation",
		Dimensions: ghtelemetry.Dimensions{
			"auth_command":                authCommand,
			"git_credential_helper":       state,
			"prefer_expiring_credentials": preferExpiringCredentials,
		},
	})
}
