package loglist

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"time"

	"github.com/google/certificate-transparency-go/loglist3"
)

// purpose is the use to which a log list will be put. This type exists to allow
// the following consts to be declared for use by LogList consumers.
type purpose string

// Issuance means that the new log list should only contain Usable logs, which
// can issue SCTs that will be trusted by all Chrome clients.
const Issuance purpose = "scts"

// Informational means that the new log list can contain Usable, Qualified, and
// Pending logs, which will all accept submissions but not necessarily be
// trusted by Chrome clients.
const Informational purpose = "info"

// Validation means that the new log list should only contain Usable and
// Readonly logs, whose SCTs will be trusted by all Chrome clients but aren't
// necessarily still issuing SCTs today.
const Validation purpose = "lint"

// List represents a list of logs arranged by the "v3" schema as published by
// Chrome: https://www.gstatic.com/ct/log_list/v3/log_list_schema.json
type List []Log

// Log represents a single log run by an operator. It contains just the info
// necessary to determine whether we want to submit to that log, and how to
// do so.
type Log struct {
	Operator       string
	Name           string
	Id             string
	Key            []byte
	Url            string
	StartInclusive time.Time
	EndExclusive   time.Time
	State          loglist3.LogStatus
	Tiled          bool
}

// usableForPurpose returns true if the log state is acceptable for the given
// log list purpose, and false otherwise.
func usableForPurpose(s loglist3.LogStatus, p purpose) bool {
	switch p {
	case Issuance:
		return s == loglist3.UsableLogStatus
	case Informational:
		return s == loglist3.UsableLogStatus || s == loglist3.QualifiedLogStatus || s == loglist3.PendingLogStatus
	case Validation:
		return s == loglist3.UsableLogStatus || s == loglist3.ReadOnlyLogStatus
	}
	return false
}

// New returns a LogList of all operators and all logs parsed from the file at
// the given path. The file must conform to the JSON Schema published by Google:
// https://www.gstatic.com/ct/log_list/v3/log_list_schema.json
func New(path string) (List, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read CT Log List: %w", err)
	}

	return newHelper(file)
}

// newHelper is a helper to allow the core logic of `New()` to be unit tested
// without having to write files to disk.
func newHelper(file []byte) (List, error) {
	parsed, err := loglist3.NewFromJSON(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CT Log List: %w", err)
	}

	result := make(List, 0)
	for _, op := range parsed.Operators {
		for _, log := range op.Logs {
			info := Log{
				Operator: op.Name,
				Name:     log.Description,
				Id:       base64.StdEncoding.EncodeToString(log.LogID),
				Key:      log.Key,
				Url:      log.URL,
				State:    log.State.LogStatus(),
				Tiled:    false,
			}

			if log.TemporalInterval != nil {
				info.StartInclusive = log.TemporalInterval.StartInclusive
				info.EndExclusive = log.TemporalInterval.EndExclusive
			}

			result = append(result, info)
		}

		for _, log := range op.TiledLogs {
			info := Log{
				Operator: op.Name,
				Name:     log.Description,
				Id:       base64.StdEncoding.EncodeToString(log.LogID),
				Key:      log.Key,
				Url:      log.SubmissionURL,
				State:    log.State.LogStatus(),
				Tiled:    true,
			}

			if log.TemporalInterval != nil {
				info.StartInclusive = log.TemporalInterval.StartInclusive
				info.EndExclusive = log.TemporalInterval.EndExclusive
			}

			result = append(result, info)
		}
	}

	return result, nil
}

// SubsetForPurpose returns a new log list containing only those logs whose
// names match those in the given list, and whose state is acceptable for the
// given purpose. It returns an error if any of the given names are not found
// in the starting list, or if the resulting list is too small to satisfy the
// Chrome "two operators" policy.
func (ll List) SubsetForPurpose(names []string, p purpose) (List, error) {
	sub, err := ll.subset(names)
	if err != nil {
		return nil, err
	}

	res, err := sub.forPurpose(p)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// subset returns a new log list containing only those logs whose names match
// those in the given list. It returns an error if any of the given names are
// not found.
func (ll List) subset(names []string) (List, error) {
	res := make(List, 0)
	for _, name := range names {
		found := false
		for _, log := range ll {
			if log.Name == name {
				if found {
					return nil, fmt.Errorf("found multiple logs matching name %q", name)
				}
				found = true
				res = append(res, log)
			}
		}
		if !found {
			return nil, fmt.Errorf("no log found matching name %q", name)
		}
	}
	return res, nil
}

// forPurpose returns a new log list containing only those logs whose states are
// acceptable for the given purpose. It returns an error if the purpose is
// Issuance or Validation and the set of remaining logs is too small to satisfy
// the Google "two operators" log policy.
func (ll List) forPurpose(p purpose) (List, error) {
	res := make(List, 0)
	operators := make(map[string]struct{})
	for _, log := range ll {
		if !usableForPurpose(log.State, p) {
			continue
		}

		res = append(res, log)
		operators[log.Operator] = struct{}{}
	}

	if len(operators) < 2 && p != Informational {
		return nil, errors.New("log list does not have enough groups to satisfy Chrome policy")
	}

	return res, nil
}

// ForTime returns a new log list containing only those logs whose temporal
// intervals include the given certificate expiration timestamp.
func (ll List) ForTime(expiry time.Time) List {
	res := slices.Clone(ll)
	res = slices.DeleteFunc(res, func(l Log) bool {
		if (l.StartInclusive.IsZero() || l.StartInclusive.Equal(expiry) || l.StartInclusive.Before(expiry)) &&
			(l.EndExclusive.IsZero() || l.EndExclusive.After(expiry)) {
			return false
		}
		return true
	})
	return res
}

// Permute returns a new log list containing the exact same logs, but in a
// randomly-shuffled order.
func (ll List) Permute() List {
	res := slices.Clone(ll)
	rand.Shuffle(len(res), func(i int, j int) {
		res[i], res[j] = res[j], res[i]
	})
	return res
}

// GetByID returns the Log matching the given ID, or an error if no such
// log can be found.
func (ll List) GetByID(logID string) (Log, error) {
	for _, log := range ll {
		if log.Id == logID {
			return log, nil
		}
	}
	return Log{}, fmt.Errorf("no log with ID %q found", logID)
}
