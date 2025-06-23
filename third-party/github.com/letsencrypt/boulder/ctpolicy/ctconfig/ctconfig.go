package ctconfig

import (
	"errors"
	"fmt"
	"time"

	"github.com/letsencrypt/boulder/config"
)

// LogShard describes a single shard of a temporally sharded
// CT log
type LogShard struct {
	URI         string
	Key         string
	WindowStart time.Time
	WindowEnd   time.Time
}

// TemporalSet contains a set of temporal shards of a single log
type TemporalSet struct {
	Name   string
	Shards []LogShard
}

// Setup initializes the TemporalSet by parsing the start and end dates
// and verifying WindowEnd > WindowStart
func (ts *TemporalSet) Setup() error {
	if ts.Name == "" {
		return errors.New("Name cannot be empty")
	}
	if len(ts.Shards) == 0 {
		return errors.New("temporal set contains no shards")
	}
	for i := range ts.Shards {
		if !ts.Shards[i].WindowEnd.After(ts.Shards[i].WindowStart) {
			return errors.New("WindowStart must be before WindowEnd")
		}
	}
	return nil
}

// pick chooses the correct shard from a TemporalSet to use for the given
// expiration time. In the case where two shards have overlapping windows
// the earlier of the two shards will be chosen.
func (ts *TemporalSet) pick(exp time.Time) (*LogShard, error) {
	for _, shard := range ts.Shards {
		if exp.Before(shard.WindowStart) {
			continue
		}
		if !exp.Before(shard.WindowEnd) {
			continue
		}
		return &shard, nil
	}
	return nil, fmt.Errorf("no valid shard available for temporal set %q for expiration date %q", ts.Name, exp)
}

// LogDescription contains the information needed to submit certificates
// to a CT log and verify returned receipts. If TemporalSet is non-nil then
// URI and Key should be empty.
type LogDescription struct {
	URI             string
	Key             string
	SubmitFinalCert bool

	*TemporalSet
}

// Info returns the URI and key of the log, either from a plain log description
// or from the earliest valid shard from a temporal log set
func (ld LogDescription) Info(exp time.Time) (string, string, error) {
	if ld.TemporalSet == nil {
		return ld.URI, ld.Key, nil
	}
	shard, err := ld.TemporalSet.pick(exp)
	if err != nil {
		return "", "", err
	}
	return shard.URI, shard.Key, nil
}

// CTGroup represents a group of CT Logs. Although capable of holding logs
// grouped by any arbitrary feature, is today primarily used to hold logs which
// are all operated by the same legal entity.
type CTGroup struct {
	Name string
	Logs []LogDescription
}

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

// LogID holds enough information to uniquely identify a CT Log: its log_id
// (the base64-encoding of the SHA-256 hash of its public key) and its human-
// readable name/description. This is used to extract other log parameters
// (such as its URL and public key) from the Chrome Log List.
type LogID struct {
	Name        string
	ID          string
	SubmitFinal bool
}
