package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCamelToKebab(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "single lowercase word",
			in:   "test",
			out:  "test",
		},
		{
			name: "multiple mixed words",
			in:   "testTestTest",
			out:  "test-test-test",
		},
		{
			name: "multiple uppercase words",
			in:   "TestTest",
			out:  "test-test",
		},
		{
			name: "multiple lowercase words",
			in:   "testtest",
			out:  "testtest",
		},
		{
			name: "multiple mixed words with number",
			in:   "test2Test",
			out:  "test2-test",
		},
		{
			name: "multiple lowercase words with number",
			in:   "test2test",
			out:  "test2test",
		},
		{
			name: "multiple lowercase words with dash",
			in:   "test-test",
			out:  "test-test",
		},
		{
			name: "multiple uppercase words with dash",
			in:   "Test-Test",
			out:  "test--test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, CamelToKebab(tt.in))
		})
	}
}
