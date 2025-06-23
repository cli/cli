package db

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestQuestionMarks(t *testing.T) {
	test.AssertEquals(t, QuestionMarks(1), "?")
	test.AssertEquals(t, QuestionMarks(2), "?,?")
	test.AssertEquals(t, QuestionMarks(3), "?,?,?")
}

func TestQuestionMarksPanic(t *testing.T) {
	defer func() { _ = recover() }()
	QuestionMarks(0)
	t.Errorf("calling QuestionMarks(0) did not panic as expected")
}
