package engine

import "strings"

// AssertionContext contains observed facts, separate from an expectation's value.
type AssertionContext struct {
	ExitCode   *int
	Text       *string
	Conclusive bool
}

// AssertionResult is shared by live results and offline evidence verification.
type AssertionResult struct {
	Status   string `json:"status"`
	Observed any    `json:"observed"`
	Reason   string `json:"reason,omitempty"`
}

// EvaluateAssertion compares one supported expectation without performing any I/O.
func EvaluateAssertion(kind string, expected any, actual AssertionContext) AssertionResult {
	result := AssertionResult{Status: "blocked"}
	switch kind {
	case "exit_code", "screen_contains", "screen_not_contains", "manual":
	default:
		result.Reason = "Unsupported expectation type: " + kind
		return result
	}
	if !actual.Conclusive {
		result.Reason = "The invocation is interrupted or incomplete; final-state assertions are not conclusive."
		return result
	}
	passed := false
	switch kind {
	case "exit_code":
		value, ok := integer(expected)
		if !ok {
			result.Reason = "Exit expectations require an integer."
			return result
		}
		if actual.ExitCode == nil {
			result.Reason = "The target's actual exit code is unavailable."
			return result
		}
		passed = *actual.ExitCode == value
		result.Observed = *actual.ExitCode
		result.Reason = "The actual target exit code was compared."
	case "screen_contains", "screen_not_contains":
		value, ok := expected.(string)
		if !ok {
			result.Reason = "Screen expectations require a string."
			return result
		}
		if actual.Text == nil {
			result.Reason = "The final visible terminal is unavailable."
			return result
		}
		found := strings.Contains(*actual.Text, value)
		passed = found == (kind == "screen_contains")
		result.Observed = map[string]bool{"found": found}
		result.Reason = "The final visible terminal was compared."
	case "manual":
		result.Reason = "This expectation needs approved independent evidence."
		return result
	}
	result.Status = "failed"
	if passed {
		result.Status = "passed"
	}
	return result
}

// AssertionVerdict preserves incomplete runs and combines verified check outcomes.
func AssertionVerdict(checks []AssertionResult, conclusive bool) string {
	if !conclusive {
		return "blocked"
	}
	blocked := false
	for _, check := range checks {
		if check.Status == "failed" {
			return "failed"
		}
		blocked = blocked || check.Status != "passed"
	}
	if blocked {
		return "blocked"
	}
	if len(checks) == 0 {
		return "observed"
	}
	return "passed"
}
