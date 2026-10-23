package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func assertionCases(t *testing.T) {
	zero, two := 0, 2
	screen := "Hello, Mona."
	for _, tc := range []struct {
		name, kind, status string
		expected           any
		actual             AssertionContext
	}{
		{name: "successful exit", kind: "exit_code", expected: 0, actual: AssertionContext{ExitCode: &zero, Conclusive: true}, status: "passed"},
		{name: "expected nonzero exit", kind: "exit_code", expected: 2, actual: AssertionContext{ExitCode: &two, Conclusive: true}, status: "passed"},
		{name: "unexpected exit", kind: "exit_code", expected: 0, actual: AssertionContext{ExitCode: &two, Conclusive: true}, status: "failed"},
		{name: "missing exit", kind: "exit_code", expected: 0, actual: AssertionContext{Conclusive: true}, status: "blocked"},
		{name: "invalid exit value", kind: "exit_code", expected: nil, actual: AssertionContext{ExitCode: &zero, Conclusive: true}, status: "blocked"},
		{name: "visible output", kind: "screen_contains", expected: "Mona", actual: AssertionContext{Text: &screen, Conclusive: true}, status: "passed"},
		{name: "missing output", kind: "screen_contains", expected: "absent", actual: AssertionContext{Text: &screen, Conclusive: true}, status: "failed"},
		{name: "forbidden output", kind: "screen_not_contains", expected: "Mona", actual: AssertionContext{Text: &screen, Conclusive: true}, status: "failed"},
		{name: "absent forbidden output", kind: "screen_not_contains", expected: "absent", actual: AssertionContext{Text: &screen, Conclusive: true}, status: "passed"},
		{name: "missing screen", kind: "screen_contains", expected: "Mona", actual: AssertionContext{Conclusive: true}, status: "blocked"},
		{name: "invalid screen expectation", kind: "screen_contains", expected: 3, actual: AssertionContext{Text: &screen, Conclusive: true}, status: "blocked"},
		{name: "incomplete exit cannot establish failure", kind: "exit_code", expected: 0, actual: AssertionContext{ExitCode: &two}, status: "blocked"},
		{name: "incomplete screen cannot establish success", kind: "screen_contains", expected: "Mona", actual: AssertionContext{Text: &screen}, status: "blocked"},
		{name: "manual evidence required", kind: "manual", actual: AssertionContext{Conclusive: true}, status: "blocked"},
		{name: "unsupported check is explicit", kind: "file_exists", actual: AssertionContext{Conclusive: true}, status: "blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := EvaluateAssertion(tc.kind, tc.expected, tc.actual)
			require.Equal(t, tc.status, result.Status)
			require.NotEmpty(t, result.Reason)
			if !tc.actual.Conclusive {
				require.Nil(t, result.Observed)
			}
		})
	}
	for _, tc := range []struct {
		name       string
		checks     []AssertionResult
		conclusive bool
		want       string
	}{
		{name: "no assertions", conclusive: true, want: "observed"},
		{name: "all pass", checks: []AssertionResult{{Status: "passed"}}, conclusive: true, want: "passed"},
		{name: "known failure", checks: []AssertionResult{{Status: "blocked"}, {Status: "failed"}}, conclusive: true, want: "failed"},
		{name: "inconclusive", checks: []AssertionResult{{Status: "passed"}, {Status: "blocked"}}, conclusive: true, want: "blocked"},
		{name: "incomplete invocation", checks: []AssertionResult{{Status: "failed"}}, want: "blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, AssertionVerdict(tc.checks, tc.conclusive))
		})
	}
}
