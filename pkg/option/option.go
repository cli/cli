// MIT License

// Copyright (c) 2022 Tom Godkin

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package o

import "fmt"

// Option represents an optional value. The [Some] variant contains a value and
// the [None] variant represents the absence of a value.
type Option[T any] struct {
	value   T
	present bool
}

// Some instantiates an [Option] with a value.
func Some[T any](value T) Option[T] {
	return Option[T]{value, true}
}

// None instantiates an [Option] with no value.
func None[T any]() Option[T] {
	return Option[T]{}
}

// String implements the [fmt.Stringer] interface.
func (o Option[T]) String() string {
	if o.present {
		return fmt.Sprintf("Some(%v)", o.value)
	}

	return "None"
}

var _ fmt.Stringer = Option[struct{}]{}

// Unwrap returns the underlying value of a [Some] variant, or panics if called
// on a [None] variant.
func (o Option[T]) Unwrap() T {
	if o.present {
		return o.value
	}

	panic("called `Option.Unwrap()` on a `None` value")
}

// UnwrapOr returns the underlying value of a [Some] variant, or the provided
// value on a [None] variant.
func (o Option[T]) UnwrapOr(value T) T {
	if o.present {
		return o.value
	}

	return value
}

// UnwrapOrElse returns the underlying value of a [Some] variant, or the result
// of calling the provided function on a [None] variant.
func (o Option[T]) UnwrapOrElse(f func() T) T {
	if o.present {
		return o.value
	}

	return f()
}

// UnwrapOrZero returns the underlying value of a [Some] variant, or the zero
// value on a [None] variant.
func (o Option[T]) UnwrapOrZero() T {
	if o.present {
		return o.value
	}

	var value T
	return value
}

// IsSome returns true if the [Option] is a [Some] variant.
func (o Option[T]) IsSome() bool {
	return o.present
}

// IsNone returns true if the [Option] is a [None] variant.
func (o Option[T]) IsNone() bool {
	return !o.present
}

// Value returns the underlying value and true for a [Some] variant, or the
// zero value and false for a [None] variant.
func (o Option[T]) Value() (T, bool) {
	return o.value, o.present
}

// Expect returns the underlying value for a [Some] variant, or panics with the
// provided message for a [None] variant.
func (o Option[T]) Expect(message string) T {
	if o.present {
		return o.value
	}

	panic(message)
}

func Map[T, U any](o Option[T], f func(T) U) Option[U] {
	if o.present {
		return Some(f(o.value))
	}

	return None[U]()
}
