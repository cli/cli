package must

// Do panics if err is not nil, otherwise returns t.
// It is useful in wrapping a two-value function call
// where you know statically that the call will succeed.
//
// Example:
//
// url := must.Do(url.Parse("http://example.com"))
func Do[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
