package auth

import (
	"testing"
)

func TestIsHostSupported(t *testing.T) {
	err := IsHostSupported()
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
}
