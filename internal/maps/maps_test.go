package maps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	t.Parallel()

	strLength := func(s string) int { return len(s) }

	input := map[string]string{"one": "a", "two": "aa", "three": "aaa"}
	expectedOutput := map[string]int{"one": 1, "two": 2, "three": 3}

	require.Equal(t, expectedOutput, Map(input, strLength))
}
