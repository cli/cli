package va

import (
	"testing"

	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test"
)

func TestReplaceInvalidUTF8(t *testing.T) {
	input := "f\xffoo"
	expected := "f\ufffdoo"
	result := replaceInvalidUTF8([]byte(input))
	if result != expected {
		t.Errorf("replaceInvalidUTF8(%q): got %q, expected %q", input, result, expected)
	}
}

func TestFilterProblemDetails(t *testing.T) {
	test.Assert(t, filterProblemDetails(nil) == nil, "nil should filter to nil")
	result := filterProblemDetails(&probs.ProblemDetails{
		Type:       probs.ProblemType([]byte{0xff, 0xfe, 0xfd}),
		Detail:     "seems okay so far whoah no \xFF\xFE\xFD",
		HTTPStatus: 999,
	})

	expected := &probs.ProblemDetails{
		Type:       "���",
		Detail:     "seems okay so far whoah no ���",
		HTTPStatus: 999,
	}
	test.AssertDeepEquals(t, result, expected)
}
