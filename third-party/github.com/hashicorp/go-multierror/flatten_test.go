package multierror

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestFlatten(t *testing.T) {
	original := &Error{
		Errors: []error{
			errors.New("one"),
			&Error{
				Errors: []error{
					errors.New("two"),
					&Error{
						Errors: []error{
							errors.New("three"),
						},
					},
				},
			},
		},
	}

	expected := `3 errors occurred:
	* one
	* two
	* three

`
	actual := fmt.Sprintf("%s", Flatten(original))

	if expected != actual {
		t.Fatalf("expected: %s, got: %s", expected, actual)
	}
}

func TestFlatten_nonError(t *testing.T) {
	err := errors.New("foo")
	actual := Flatten(err)
	if !reflect.DeepEqual(actual, err) {
		t.Fatalf("bad: %#v", actual)
	}
}
