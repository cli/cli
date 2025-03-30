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
package o_test

import (
	"fmt"
	"testing"

	o "github.com/cli/cli/v2/pkg/option"
	"github.com/stretchr/testify/require"
)

func ExampleOption_Unwrap() {
	fmt.Println(o.Some(4).Unwrap())
	// Output: 4
}

func ExampleOption_UnwrapOr() {
	fmt.Println(o.Some(4).UnwrapOr(3))
	fmt.Println(o.None[int]().UnwrapOr(3))
	// Output:
	// 4
	// 3
}

func ExampleOption_UnwrapOrElse() {
	fmt.Println(o.Some(4).UnwrapOrElse(func() int {
		return 3
	}))

	fmt.Println(o.None[int]().UnwrapOrElse(func() int {
		return 3
	}))

	// Output:
	// 4
	// 3
}

func ExampleOption_UnwrapOrZero() {
	fmt.Println(o.Some(4).UnwrapOrZero())
	fmt.Println(o.None[int]().UnwrapOrZero())

	// Output
	// 4
	// 0
}

func ExampleOption_IsSome() {
	fmt.Println(o.Some(4).IsSome())
	fmt.Println(o.None[int]().IsSome())

	// Output:
	// true
	// false
}

func ExampleOption_IsNone() {
	fmt.Println(o.Some(4).IsNone())
	fmt.Println(o.None[int]().IsNone())

	// Output:
	// false
	// true
}

func ExampleOption_Value() {
	value, ok := o.Some(4).Value()
	fmt.Println(value)
	fmt.Println(ok)

	// Output:
	// 4
	// true
}

func ExampleOption_Expect() {
	fmt.Println(o.Some(4).Expect("oops"))

	// Output: 4
}

func ExampleMap() {
	fmt.Println(o.Map(o.Some(2), double))
	fmt.Println(o.Map(o.None[int](), double))

	// Output:
	// Some(4)
	// None
}

func double(i int) int {
	return i * 2
}

func TestSomeStringer(t *testing.T) {
	require.Equal(t, fmt.Sprintf("%s", o.Some("foo")), "Some(foo)") //nolint:gosimple
	require.Equal(t, fmt.Sprintf("%s", o.Some(42)), "Some(42)")     //nolint:gosimple
}

func TestNoneStringer(t *testing.T) {
	require.Equal(t, fmt.Sprintf("%s", o.None[string]()), "None") //nolint:gosimple
}

func TestSomeUnwrap(t *testing.T) {
	require.Equal(t, o.Some(42).Unwrap(), 42)
}

func TestNoneUnwrap(t *testing.T) {
	defer func() {
		require.Equal(t, fmt.Sprint(recover()), "called `Option.Unwrap()` on a `None` value")
	}()

	o.None[string]().Unwrap()
	t.Error("did not panic")
}

func TestSomeUnwrapOr(t *testing.T) {
	require.Equal(t, o.Some(42).UnwrapOr(3), 42)
}

func TestNoneUnwrapOr(t *testing.T) {
	require.Equal(t, o.None[int]().UnwrapOr(3), 3)
}

func TestSomeUnwrapOrElse(t *testing.T) {
	require.Equal(t, o.Some(42).UnwrapOrElse(func() int { return 41 }), 42)
}

func TestNoneUnwrapOrElse(t *testing.T) {
	require.Equal(t, o.None[int]().UnwrapOrElse(func() int { return 41 }), 41)
}

func TestSomeUnwrapOrZero(t *testing.T) {
	require.Equal(t, o.Some(42).UnwrapOrZero(), 42)
}

func TestNoneUnwrapOrZero(t *testing.T) {
	require.Equal(t, o.None[int]().UnwrapOrZero(), 0)
}

func TestIsSome(t *testing.T) {
	require.True(t, o.Some(42).IsSome())
	require.False(t, o.None[int]().IsSome())
}

func TestIsNone(t *testing.T) {
	require.False(t, o.Some(42).IsNone())
	require.True(t, o.None[int]().IsNone())
}

func TestSomeValue(t *testing.T) {
	value, ok := o.Some(42).Value()
	require.Equal(t, value, 42)
	require.True(t, ok)
}

func TestNoneValue(t *testing.T) {
	value, ok := o.None[int]().Value()
	require.Equal(t, value, 0)
	require.False(t, ok)
}

func TestSomeExpect(t *testing.T) {
	require.Equal(t, o.Some(42).Expect("oops"), 42)
}

func TestNoneExpect(t *testing.T) {
	defer func() {
		require.Equal(t, fmt.Sprint(recover()), "oops")
	}()

	o.None[int]().Expect("oops")
	t.Error("did not panic")
}

func TestMap(t *testing.T) {
	require.Equal(t, o.Map(o.Some(2), double), o.Some(4))
	require.True(t, o.Map(o.None[int](), double).IsNone())
}
