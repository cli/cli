package web

import (
	"errors"
	"net/http/httptest"
	"testing"

	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test"
)

func TestSendErrorSubProblemNamespace(t *testing.T) {
	rw := httptest.NewRecorder()
	prob := ProblemDetailsForError((&berrors.BoulderError{
		Type:   berrors.Malformed,
		Detail: "bad",
	}).WithSubErrors(
		[]berrors.SubBoulderError{
			{
				Identifier: identifier.NewDNS("example.com"),
				BoulderError: &berrors.BoulderError{
					Type:   berrors.Malformed,
					Detail: "nop",
				},
			},
			{
				Identifier: identifier.NewDNS("what about example.com"),
				BoulderError: &berrors.BoulderError{
					Type:   berrors.Malformed,
					Detail: "nah",
				},
			},
		}),
		"dfoop",
	)
	SendError(log.NewMock(), rw, &RequestEvent{}, prob, errors.New("it bad"))

	body := rw.Body.String()
	test.AssertUnmarshaledEquals(t, body, `{
		"type": "urn:ietf:params:acme:error:malformed",
		"detail": "dfoop :: bad",
		"status": 400,
		"subproblems": [
		  {
			"type": "urn:ietf:params:acme:error:malformed",
			"detail": "dfoop :: nop",
			"status": 400,
			"identifier": {
			  "type": "dns",
			  "value": "example.com"
			}
		  },
		  {
			"type": "urn:ietf:params:acme:error:malformed",
			"detail": "dfoop :: nah",
			"status": 400,
			"identifier": {
			  "type": "dns",
			  "value": "what about example.com"
			}
		  }
		]
	  }`)
}

func TestSendErrorSubProbLogging(t *testing.T) {
	rw := httptest.NewRecorder()
	prob := ProblemDetailsForError((&berrors.BoulderError{
		Type:   berrors.Malformed,
		Detail: "bad",
	}).WithSubErrors(
		[]berrors.SubBoulderError{
			{
				Identifier: identifier.NewDNS("example.com"),
				BoulderError: &berrors.BoulderError{
					Type:   berrors.Malformed,
					Detail: "nop",
				},
			},
			{
				Identifier: identifier.NewDNS("what about example.com"),
				BoulderError: &berrors.BoulderError{
					Type:   berrors.Malformed,
					Detail: "nah",
				},
			},
		}),
		"dfoop",
	)
	logEvent := RequestEvent{}
	SendError(log.NewMock(), rw, &logEvent, prob, errors.New("it bad"))

	test.AssertEquals(t, logEvent.Error, `400 :: malformed :: dfoop :: bad ["example.com :: malformed :: dfoop :: nop", "what about example.com :: malformed :: dfoop :: nah"]`)
}

func TestSendErrorPausedProblemLoggingSuppression(t *testing.T) {
	rw := httptest.NewRecorder()
	logEvent := RequestEvent{}
	SendError(log.NewMock(), rw, &logEvent, probs.Paused("I better not see any of this"), nil)

	test.AssertEquals(t, logEvent.Error, "429 :: rateLimited :: account/ident pair is paused")
}
