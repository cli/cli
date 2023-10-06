package queryfilter

import (
	"fmt"
	"regexp"

	"github.com/TheCount/go-structfilter/structfilter"
)

func FilterTemplate(a any) (any, error) {
	filter := structfilter.New(
		structfilter.RemoveFieldFilter(regexp.MustCompile("^Template$")),
	)
	filtered, err := filter.Convert(a)
	if err != nil {
		var zero any
		return zero, fmt.Errorf("failed to remove Template from struct: %v", err)
	}

	return filtered, nil
}

func None(a any) (any, error) {
	return a, nil
}
