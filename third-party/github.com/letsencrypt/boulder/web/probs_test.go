package web

import (
	"fmt"
	"reflect"
	"testing"

	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test"
)

func TestProblemDetailsFromError(t *testing.T) {
	// errMsg is used as the msg argument for `ProblemDetailsForError` and is
	// always returned in the problem detail.
	const errMsg = "testError"
	// detailMsg is used as the msg argument for the individual error types and is
	// sometimes not present in the produced problem's detail.
	const detailMsg = "testDetail"
	// fullDetail is what we expect the problem detail to look like when it
	// contains both the error message and the detail message
	fullDetail := fmt.Sprintf("%s :: %s", errMsg, detailMsg)
	testCases := []struct {
		err        error
		statusCode int
		problem    probs.ProblemType
		detail     string
	}{
		// boulder/errors error types
		//   Internal server errors expect just the `errMsg` in detail.
		{berrors.InternalServerError(detailMsg), 500, probs.ServerInternalProblem, errMsg},
		//   Other errors expect the full detail message
		{berrors.MalformedError(detailMsg), 400, probs.MalformedProblem, fullDetail},
		{berrors.UnauthorizedError(detailMsg), 403, probs.UnauthorizedProblem, fullDetail},
		{berrors.NotFoundError(detailMsg), 404, probs.MalformedProblem, fullDetail},
		{berrors.RateLimitError(0, detailMsg), 429, probs.RateLimitedProblem, fullDetail + ": see https://letsencrypt.org/docs/rate-limits/"},
		{berrors.InvalidEmailError(detailMsg), 400, probs.InvalidContactProblem, fullDetail},
		{berrors.RejectedIdentifierError(detailMsg), 400, probs.RejectedIdentifierProblem, fullDetail},
	}
	for _, c := range testCases {
		p := ProblemDetailsForError(c.err, errMsg)
		if p.HTTPStatus != c.statusCode {
			t.Errorf("Incorrect status code for %s. Expected %d, got %d", reflect.TypeOf(c.err).Name(), c.statusCode, p.HTTPStatus)
		}
		if p.Type != c.problem {
			t.Errorf("Expected problem urn %#v, got %#v", c.problem, p.Type)
		}
		if p.Detail != c.detail {
			t.Errorf("Expected detailed message %q, got %q", c.detail, p.Detail)
		}
	}

	expected := &probs.ProblemDetails{
		Type:       probs.MalformedProblem,
		HTTPStatus: 200,
		Detail:     "gotcha",
	}
	p := ProblemDetailsForError(expected, "k")
	test.AssertDeepEquals(t, expected, p)
}

func TestSubProblems(t *testing.T) {
	topErr := (&berrors.BoulderError{
		Type:   berrors.CAA,
		Detail: "CAA policy forbids issuance",
	}).WithSubErrors(
		[]berrors.SubBoulderError{
			{
				Identifier: identifier.DNSIdentifier("threeletter.agency"),
				BoulderError: &berrors.BoulderError{
					Type:   berrors.CAA,
					Detail: "Forbidden by ■■■■■■■■■■■ and directive ■■■■",
				},
			},
			{
				Identifier: identifier.DNSIdentifier("area51.threeletter.agency"),
				BoulderError: &berrors.BoulderError{
					Type:   berrors.NotFound,
					Detail: "No Such Area...",
				},
			},
		})

	prob := problemDetailsForBoulderError(topErr, "problem with subproblems")
	test.AssertEquals(t, len(prob.SubProblems), len(topErr.SubErrors))

	subProbsMap := make(map[string]probs.SubProblemDetails, len(prob.SubProblems))

	for _, subProb := range prob.SubProblems {
		subProbsMap[subProb.Identifier.Value] = subProb
	}

	subProbA, foundA := subProbsMap["threeletter.agency"]
	subProbB, foundB := subProbsMap["area51.threeletter.agency"]
	test.AssertEquals(t, foundA, true)
	test.AssertEquals(t, foundB, true)

	test.AssertEquals(t, subProbA.Type, probs.CAAProblem)
	test.AssertEquals(t, subProbB.Type, probs.MalformedProblem)
}
