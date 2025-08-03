package db

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestValidMariaDBUnquotedIdentifier(t *testing.T) {
	test.AssertError(t, validMariaDBUnquotedIdentifier("12345"), "expected error for 12345")
	test.AssertError(t, validMariaDBUnquotedIdentifier("12345e"), "expected error for 12345e")
	test.AssertError(t, validMariaDBUnquotedIdentifier("1e10"), "expected error for 1e10")
	test.AssertError(t, validMariaDBUnquotedIdentifier("foo\\bar"), "expected error for foo\\bar")
	test.AssertError(t, validMariaDBUnquotedIdentifier("zoom "), "expected error for identifier ending in space")
	test.AssertNotError(t, validMariaDBUnquotedIdentifier("hi"), "expected no error for 'hi'")
}
