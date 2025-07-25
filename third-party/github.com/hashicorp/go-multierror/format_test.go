package multierror

import (
	"errors"
	"testing"
)

func TestListFormatFuncSingle(t *testing.T) {
	expected := `1 error occurred:
	* foo

`

	errors := []error{
		errors.New("foo"),
	}

	actual := ListFormatFunc(errors)
	if actual != expected {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestListFormatFuncMultiple(t *testing.T) {
	expected := `2 errors occurred:
	* foo
	* bar

`

	errors := []error{
		errors.New("foo"),
		errors.New("bar"),
	}

	actual := ListFormatFunc(errors)
	if actual != expected {
		t.Fatalf("bad: %#v", actual)
	}
}
