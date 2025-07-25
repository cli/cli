package allowlist

import (
	"testing"
)

func TestNewFromYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		yamlData      string
		check         []string
		expectAnswers []bool
		expectErr     bool
	}{
		{
			name:          "valid YAML",
			yamlData:      "- oak\n- maple\n- cherry",
			check:         []string{"oak", "walnut", "maple", "cherry"},
			expectAnswers: []bool{true, false, true, true},
			expectErr:     false,
		},
		{
			name:          "empty YAML",
			yamlData:      "",
			check:         []string{"oak", "walnut", "maple", "cherry"},
			expectAnswers: []bool{false, false, false, false},
			expectErr:     false,
		},
		{
			name:          "invalid YAML",
			yamlData:      "{ invalid_yaml",
			check:         []string{},
			expectAnswers: []bool{},
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			list, err := NewFromYAML[string]([]byte(tt.yamlData))
			if (err != nil) != tt.expectErr {
				t.Fatalf("NewFromYAML() error = %v, expectErr = %v", err, tt.expectErr)
			}

			if err == nil {
				for i, item := range tt.check {
					got := list.Contains(item)
					if got != tt.expectAnswers[i] {
						t.Errorf("Contains(%q) got %v, want %v", item, got, tt.expectAnswers[i])
					}
				}
			}
		})
	}
}

func TestNewList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		members       []string
		check         []string
		expectAnswers []bool
	}{
		{
			name:          "unique members",
			members:       []string{"oak", "maple", "cherry"},
			check:         []string{"oak", "walnut", "maple", "cherry"},
			expectAnswers: []bool{true, false, true, true},
		},
		{
			name:          "duplicate members",
			members:       []string{"oak", "maple", "cherry", "oak"},
			check:         []string{"oak", "walnut", "maple", "cherry"},
			expectAnswers: []bool{true, false, true, true},
		},
		{
			name:          "nil list",
			members:       nil,
			check:         []string{"oak", "walnut", "maple", "cherry"},
			expectAnswers: []bool{false, false, false, false},
		},
		{
			name:          "empty list",
			members:       []string{},
			check:         []string{"oak", "walnut", "maple", "cherry"},
			expectAnswers: []bool{false, false, false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			list := NewList[string](tt.members)
			for i, item := range tt.check {
				got := list.Contains(item)
				if got != tt.expectAnswers[i] {
					t.Errorf("Contains(%q) got %v, want %v", item, got, tt.expectAnswers[i])
				}
			}
		})
	}
}
