package ghenv

import "testing"

func TestReadOnly(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "1", want: true},
		{value: "true", want: true},
		{value: "yes", want: true},
		{value: "", want: false},
		{value: "0", want: false},
		{value: "false", want: false},
		{value: "no", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("GH_READ_ONLY", tt.value)
			if got := ReadOnly(); got != tt.want {
				t.Errorf("ReadOnly() with GH_READ_ONLY=%q = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
