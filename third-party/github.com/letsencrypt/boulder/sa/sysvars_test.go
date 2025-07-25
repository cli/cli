package sa

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestCheckMariaDBSystemVariables(t *testing.T) {
	type testCase struct {
		key       string
		value     string
		expectErr string
	}

	for _, tc := range []testCase{
		{"sql_select_limit", "'0.1", "requires a numeric value"},
		{"max_statement_time", "0", ""},
		{"myBabies", "kids_I_tell_ya", "was unexpected"},
		{"sql_mode", "'STRICT_ALL_TABLES", "string is not properly quoted"},
		{"sql_mode", "%27STRICT_ALL_TABLES%27", "string is not properly quoted"},
		{"completion_type", "1", ""},
		{"completion_type", "'2'", "integer enum is quoted, but should not be"},
		{"completion_type", "RELEASE", "string enum is not properly quoted"},
		{"completion_type", "'CHAIN'", ""},
		{"autocommit", "0", ""},
		{"check_constraint_checks", "1", ""},
		{"log_slow_query", "true", ""},
		{"foreign_key_checks", "false", ""},
		{"sql_warnings", "TrUe", ""},
		{"tx_read_only", "FalSe", ""},
		{"sql_notes", "on", ""},
		{"tcp_nodelay", "off", ""},
		{"autocommit", "2", "expected boolean value"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			err := checkMariaDBSystemVariables(tc.key, tc.value)
			if tc.expectErr == "" {
				test.AssertNotError(t, err, "Unexpected error received")
			} else {
				test.AssertError(t, err, "Error expected, but not found")
				test.AssertContains(t, err.Error(), tc.expectErr)
			}
		})
	}
}
