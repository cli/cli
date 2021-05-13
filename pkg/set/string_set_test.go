package set

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_StringSlice_ToSlice(t *testing.T) {
	s := NewStringSet()
	s.Add("one")
	s.Add("two")
	s.Add("three")
	s.Add("two")
	assert.Equal(t, []string{"one", "two", "three"}, s.ToSlice())
}

func Test_StringSlice_Remove(t *testing.T) {
	s := NewStringSet()
	s.Add("one")
	s.Add("two")
	s.Add("three")
	s.Remove("two")
	assert.Equal(t, []string{"one", "three"}, s.ToSlice())
	assert.False(t, s.Contains("two"))
	assert.Equal(t, 2, s.Len())
}
