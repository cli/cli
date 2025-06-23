package multierror

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestError_Impl(t *testing.T) {
	var _ error = new(Error)
}

func TestErrorError_custom(t *testing.T) {
	errors := []error{
		errors.New("foo"),
		errors.New("bar"),
	}

	fn := func(es []error) string {
		return "foo"
	}

	multi := &Error{Errors: errors, ErrorFormat: fn}
	if multi.Error() != "foo" {
		t.Fatalf("bad: %s", multi.Error())
	}
}

func TestErrorError_default(t *testing.T) {
	expected := `2 errors occurred:
	* foo
	* bar

`

	errors := []error{
		errors.New("foo"),
		errors.New("bar"),
	}

	multi := &Error{Errors: errors}
	if multi.Error() != expected {
		t.Fatalf("bad: %s", multi.Error())
	}
}

func TestErrorErrorOrNil(t *testing.T) {
	err := new(Error)
	if err.ErrorOrNil() != nil {
		t.Fatalf("bad: %#v", err.ErrorOrNil())
	}

	err.Errors = []error{errors.New("foo")}
	if v := err.ErrorOrNil(); v == nil {
		t.Fatal("should not be nil")
	} else if !reflect.DeepEqual(v, err) {
		t.Fatalf("bad: %#v", v)
	}
}

func TestErrorWrappedErrors(t *testing.T) {
	errors := []error{
		errors.New("foo"),
		errors.New("bar"),
	}

	multi := &Error{Errors: errors}
	if !reflect.DeepEqual(multi.Errors, multi.WrappedErrors()) {
		t.Fatalf("bad: %s", multi.WrappedErrors())
	}

	multi = nil
	if err := multi.WrappedErrors(); err != nil {
		t.Fatalf("bad: %#v", multi)
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Run("with errors", func(t *testing.T) {
		err := &Error{Errors: []error{
			errors.New("foo"),
			errors.New("bar"),
			errors.New("baz"),
		}}

		var current error = err
		for i := 0; i < len(err.Errors); i++ {
			current = errors.Unwrap(current)
			if !errors.Is(current, err.Errors[i]) {
				t.Fatal("should be next value")
			}
		}

		if errors.Unwrap(current) != nil {
			t.Fatal("should be nil at the end")
		}
	})

	t.Run("with no errors", func(t *testing.T) {
		err := &Error{Errors: nil}
		if errors.Unwrap(err) != nil {
			t.Fatal("should be nil")
		}
	})

	t.Run("with nil multierror", func(t *testing.T) {
		var err *Error
		if errors.Unwrap(err) != nil {
			t.Fatal("should be nil")
		}
	})
}

func TestErrorIs(t *testing.T) {
	errBar := errors.New("bar")

	t.Run("with errBar", func(t *testing.T) {
		err := &Error{Errors: []error{
			errors.New("foo"),
			errBar,
			errors.New("baz"),
		}}

		if !errors.Is(err, errBar) {
			t.Fatal("should be true")
		}
	})

	t.Run("with errBar wrapped by fmt.Errorf", func(t *testing.T) {
		err := &Error{Errors: []error{
			errors.New("foo"),
			fmt.Errorf("errorf: %w", errBar),
			errors.New("baz"),
		}}

		if !errors.Is(err, errBar) {
			t.Fatal("should be true")
		}
	})

	t.Run("without errBar", func(t *testing.T) {
		err := &Error{Errors: []error{
			errors.New("foo"),
			errors.New("baz"),
		}}

		if errors.Is(err, errBar) {
			t.Fatal("should be false")
		}
	})
}

func TestErrorAs(t *testing.T) {
	match := &nestedError{}

	t.Run("with the value", func(t *testing.T) {
		err := &Error{Errors: []error{
			errors.New("foo"),
			match,
			errors.New("baz"),
		}}

		var target *nestedError
		if !errors.As(err, &target) {
			t.Fatal("should be true")
		}
		if target == nil {
			t.Fatal("target should not be nil")
		}
	})

	t.Run("with the value wrapped by fmt.Errorf", func(t *testing.T) {
		err := &Error{Errors: []error{
			errors.New("foo"),
			fmt.Errorf("errorf: %w", match),
			errors.New("baz"),
		}}

		var target *nestedError
		if !errors.As(err, &target) {
			t.Fatal("should be true")
		}
		if target == nil {
			t.Fatal("target should not be nil")
		}
	})

	t.Run("without the value", func(t *testing.T) {
		err := &Error{Errors: []error{
			errors.New("foo"),
			errors.New("baz"),
		}}

		var target *nestedError
		if errors.As(err, &target) {
			t.Fatal("should be false")
		}
		if target != nil {
			t.Fatal("target should be nil")
		}
	})
}

// nestedError implements error and is used for tests.
type nestedError struct{}

func (*nestedError) Error() string { return "" }
