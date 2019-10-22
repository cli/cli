package survey

import (
	"reflect"
	"strings"
)

// TransformString returns a `Transformer` based on the "f"
// function which accepts a string representation of the answer
// and returns a new one, transformed, answer.
// Take for example the functions inside the std `strings` package,
// they can be converted to a compatible `Transformer` by using this function,
// i.e: `TransformString(strings.Title)`, `TransformString(strings.ToUpper)`.
//
// Note that `TransformString` is just a helper, `Transformer` can be used
// to transform any type of answer.
func TransformString(f func(s string) string) Transformer {
	return func(ans interface{}) interface{} {
		// if the answer value passed in is the zero value of the appropriate type
		if isZero(reflect.ValueOf(ans)) {
			// skip this `Transformer` by returning a nil value.
			// The original answer will be not affected,
			// see survey.go#L125.
			return nil
		}

		// "ans" is never nil here, so we don't have to check that
		// see survey.go#L97 for more.
		// Make sure that the the answer's value was a typeof string.
		s, ok := ans.(string)
		if !ok {
			return nil
		}

		return f(s)
	}
}

// ToLower is a `Transformer`.
// It receives an answer value
// and returns a copy of the "ans"
// with all Unicode letters mapped to their lower case.
//
// Note that if "ans" is not a string then it will
// return a nil value, meaning that the above answer
// will not be affected by this call at all.
func ToLower(ans interface{}) interface{} {
	transformer := TransformString(strings.ToLower)
	return transformer(ans)
}

// Title is a `Transformer`.
// It receives an answer value
// and returns a copy of the "ans"
// with all Unicode letters that begin words
// mapped to their title case.
//
// Note that if "ans" is not a string then it will
// return a nil value, meaning that the above answer
// will not be affected by this call at all.
func Title(ans interface{}) interface{} {
	transformer := TransformString(strings.Title)
	return transformer(ans)
}

// ComposeTransformers is a variadic function used to create one transformer from many.
func ComposeTransformers(transformers ...Transformer) Transformer {
	// return a transformer that calls each one sequentially
	return func(ans interface{}) interface{} {
		// execute each transformer
		for _, t := range transformers {
			ans = t(ans)
		}
		return ans
	}
}
