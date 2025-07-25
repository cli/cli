package va

import (
	"strings"
	"unicode/utf8"

	"github.com/letsencrypt/boulder/probs"
)

// replaceInvalidUTF8 replaces all invalid UTF-8 encodings with
// Unicode REPLACEMENT CHARACTER.
func replaceInvalidUTF8(input []byte) string {
	if utf8.Valid(input) {
		return string(input)
	}

	var b strings.Builder

	// Ranging over a string in Go produces runes. When the range keyword
	// encounters an invalid UTF-8 encoding, it returns REPLACEMENT CHARACTER.
	for _, v := range string(input) {
		b.WriteRune(v)
	}
	return b.String()
}

// Call replaceInvalidUTF8 on all string fields of a ProblemDetails
// and return the result.
func filterProblemDetails(prob *probs.ProblemDetails) *probs.ProblemDetails {
	if prob == nil {
		return nil
	}
	return &probs.ProblemDetails{
		Type:       probs.ProblemType(replaceInvalidUTF8([]byte(prob.Type))),
		Detail:     replaceInvalidUTF8([]byte(prob.Detail)),
		HTTPStatus: prob.HTTPStatus,
	}
}
