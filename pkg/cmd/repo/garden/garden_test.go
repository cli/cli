package garden

import (
	"testing"
)

func TestComputeSeed_ShortName(t *testing.T) {
	// Should not panic on short repository names like "a/b"
	seed := computeSeed("a/b")
	if seed == 0 {
		t.Errorf("expected non-zero seed, got 0")
	}
}

func TestShaToColorFunc_ShortSha(t *testing.T) {
	// Should not panic on short or empty SHAs
	colorFn := shaToColorFunc("abc")
	output := colorFn("x")
	if output == "" {
		t.Errorf("expected formatted output, got empty string")
	}
}
