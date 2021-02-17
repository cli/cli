package set

var exists = struct{}{}

type stringSet struct {
	m map[string]struct{}
}

func NewStringSet() *stringSet {
	s := &stringSet{}
	s.m = make(map[string]struct{})
	return s
}

func (s *stringSet) Add(value string) {
	s.m[value] = exists
}

func (s *stringSet) AddValues(values []string) {
	for _, v := range values {
		s.Add(v)
	}
}

func (s *stringSet) Remove(value string) {
	delete(s.m, value)
}

func (s *stringSet) RemoveValues(values []string) {
	for _, v := range values {
		s.Remove(v)
	}
}

func (s *stringSet) Contains(value string) bool {
	_, c := s.m[value]
	return c
}

func (s *stringSet) ToSlice() []string {
	r := make([]string, 0, len(s.m))
	for k := range s.m {
		r = append(r, k)
	}
	return r
}
