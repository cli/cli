package multierror

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestSortSingle(t *testing.T) {
	errFoo := errors.New("foo")

	expected := []error{
		errFoo,
	}

	err := &Error{
		Errors: []error{
			errFoo,
		},
	}

	sort.Sort(err)
	if !reflect.DeepEqual(err.Errors, expected) {
		t.Fatalf("bad: %#v", err)
	}
}

func TestSortMultiple(t *testing.T) {
	errBar := errors.New("bar")
	errBaz := errors.New("baz")
	errFoo := errors.New("foo")

	expected := []error{
		errBar,
		errBaz,
		errFoo,
	}

	err := &Error{
		Errors: []error{
			errFoo,
			errBar,
			errBaz,
		},
	}

	sort.Sort(err)
	if !reflect.DeepEqual(err.Errors, expected) {
		t.Fatalf("bad: %#v", err)
	}
}
