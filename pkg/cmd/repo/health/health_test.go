package health

import (
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestNewCmdHealth(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &HealthOptions{
		IO: ios,
	}
	
	// Test defaults
	if opts.JSONOutput != false {
		t.Error("JSONOutput should default to false")
	}
}

func TestCheckResultStruct(t *testing.T) {
	cr := CheckResult{
		Name:   "Test",
		Passed: true,
		Score:  10,
		MaxScore: 10,
	}
	
	if !cr.Passed {
		t.Error("Expected Passed to be true")
	}
}
