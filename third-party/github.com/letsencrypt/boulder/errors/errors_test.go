package errors

import (
	"testing"

	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/test"
)

// TestWithSubErrors tests that a boulder error can be created by adding
// suberrors to an existing top level boulder error
func TestWithSubErrors(t *testing.T) {
	topErr := &BoulderError{
		Type:   RateLimit,
		Detail: "don't you think you have enough certificates already?",
	}

	subErrs := []SubBoulderError{
		{
			Identifier: identifier.DNSIdentifier("example.com"),
			BoulderError: &BoulderError{
				Type:   RateLimit,
				Detail: "everyone uses this example domain",
			},
		},
		{
			Identifier: identifier.DNSIdentifier("what about example.com"),
			BoulderError: &BoulderError{
				Type:   RateLimit,
				Detail: "try a real identifier value next time",
			},
		},
	}

	outResult := topErr.WithSubErrors(subErrs)
	// The outResult should be a new, distinct error
	test.AssertNotEquals(t, topErr, outResult)
	// The outResult error should have the correct sub errors
	test.AssertDeepEquals(t, outResult.SubErrors, subErrs)
	// Adding another suberr shouldn't squash the original sub errors
	anotherSubErr := SubBoulderError{
		Identifier: identifier.DNSIdentifier("another ident"),
		BoulderError: &BoulderError{
			Type:   RateLimit,
			Detail: "another rate limit err",
		},
	}
	outResult = outResult.WithSubErrors([]SubBoulderError{anotherSubErr})
	test.AssertDeepEquals(t, outResult.SubErrors, append(subErrs, anotherSubErr))
}
