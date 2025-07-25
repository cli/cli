package ctconfig

import (
	"github.com/letsencrypt/boulder/config"
)

// CTConfig is the top-level config object expected to be embedded in an
// executable's JSON config struct.
type CTConfig struct {
	// Stagger is duration (e.g. "200ms") indicating how long to wait for a log
	// from one operator group to accept a certificate before attempting
	// submission to a log run by a different operator instead.
	Stagger config.Duration
	// LogListFile is a path to a JSON log list file. The file must match Chrome's
	// schema: https://www.gstatic.com/ct/log_list/v3/log_list_schema.json
	LogListFile string `validate:"required"`
	// SCTLogs is a list of CT log names to submit precerts to in order to get SCTs.
	SCTLogs []string `validate:"min=1,dive,required"`
	// InfoLogs is a list of CT log names to submit precerts to on a best-effort
	// basis. Logs are included here for the sake of wider distribution of our
	// precerts, and to exercise logs that in the qualification process.
	InfoLogs []string
	// FinalLogs is a list of CT log names to submit final certificates to.
	// This may include duplicates from the lists above, to submit both precerts
	// and final certs to the same log.
	FinalLogs []string
}
