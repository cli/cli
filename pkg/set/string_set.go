package set

var exists = struct{}{}

type stringSet struct {
	v []string
	m map[string]struct{}
}

func NewStringSet() *stringSet {
	s := &stringSet{}
	s.m = make(map[string]struct{})
	s.v = []string{}
	return s
}

func NewFromSlice(sl []string) *stringSet {
	s := NewStringSet()
	s.AddValues(sl)
	return s
}

func (s *stringSet) Add(value string) {
	if s.Contains(value) {
		return
	}
	s.m[value] = exists
	s.v = append(s.v, value)
}

func (s *stringSet) AddValues(values []string) {
	for _, v := range values {
		s.Add(v)
	}
}

func (s *stringSet) Remove(value string) {
	if !s.Contains(value) {
		return
	}
	delete(s.m, value)
	s.v = sliceWithout(s.v, value)
}

func (s1 *stringSet) Difference(s2 *stringSet) *stringSet {
	s := NewStringSet()
	for _, v := range s1.v {
		if !s2.Contains(v) {
			s.Add(v)
		}
	}
	return s
}

func sliceWithout(s []string, v string) []string {
	idx := -1
	for i, item := range s {
		if item == v {
			idx = i
			break
		}
	}
	if idx < 0 {
		return s
	}
	return append(s[:idx], s[idx+1:]...)
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

func (s *stringSet) Len() int {
	return len(s.m)
}

func (s *stringSet) ToSlice() []string {
	return s.v
}

func (s1 *stringSet) Equal(s2 *stringSet) bool {
	if s1.Len() != s2.Len() {
		return false
	}
	isEqual := true
	for _, v := range s1.v {
		if !s2.Contains(v) {
			isEqual = false
			break
		}
	}
	return isEqual
}
