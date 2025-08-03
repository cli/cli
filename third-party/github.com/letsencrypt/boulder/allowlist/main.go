package allowlist

import (
	"github.com/letsencrypt/boulder/strictyaml"
)

// List holds a unique collection of items of type T. Membership can be checked
// by calling the Contains method.
type List[T comparable] struct {
	members map[T]struct{}
}

// NewList returns a *List[T] populated with the provided members of type T. All
// duplicate entries are ignored, ensuring uniqueness.
func NewList[T comparable](members []T) *List[T] {
	l := &List[T]{members: make(map[T]struct{})}
	for _, m := range members {
		l.members[m] = struct{}{}
	}
	return l
}

// NewFromYAML reads a YAML sequence of values of type T and returns a *List[T]
// containing those values. If data is empty, an empty (deny all) list is
// returned. If data cannot be parsed, an error is returned.
func NewFromYAML[T comparable](data []byte) (*List[T], error) {
	if len(data) == 0 {
		return NewList([]T{}), nil
	}

	var entries []T
	err := strictyaml.Unmarshal(data, &entries)
	if err != nil {
		return nil, err
	}
	return NewList(entries), nil
}

// Contains reports whether the provided entry is a member of the list.
func (l *List[T]) Contains(entry T) bool {
	_, ok := l.members[entry]
	return ok
}
